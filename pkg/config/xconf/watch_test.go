package xconf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Watch 单元测试
// =============================================================================

func TestWatch_Success(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	initialContent := `app:
  name: test
  version: "1.0"
`
	err := os.WriteFile(configPath, []byte(initialContent), 0600)
	require.NoError(t, err)

	// 加载配置
	cfg, err := New(configPath)
	require.NoError(t, err)

	// 验证初始值
	assert.Equal(t, "test", cfg.Client().String("app.name"))

	// 创建监视器
	var mu sync.Mutex
	var reloadCount int
	var lastErr error

	w, err := Watch(cfg, func(c Config, err error) {
		mu.Lock()
		defer mu.Unlock()
		reloadCount++
		lastErr = err
	})
	require.NoError(t, err)

	// 异步启动监视
	w.StartAsync()
	defer func() { _ = w.Stop() }()

	// 等待监视器启动
	time.Sleep(50 * time.Millisecond)

	// 修改配置文件
	newContent := `app:
  name: updated
  version: "2.0"
`
	err = os.WriteFile(configPath, []byte(newContent), 0600)
	require.NoError(t, err)

	// 等待重载（防抖 100ms + 一些延迟）
	time.Sleep(200 * time.Millisecond)

	// 验证回调被调用
	mu.Lock()
	assert.GreaterOrEqual(t, reloadCount, 1, "callback should be called at least once")
	assert.NoError(t, lastErr, "reload should not error")
	mu.Unlock()

	// 验证配置已更新
	assert.Equal(t, "updated", cfg.Client().String("app.name"))
}

func TestWatch_FromBytes_Error(t *testing.T) {
	// 从 bytes 创建的配置不支持监视
	data := []byte(`app:
  name: test
`)
	cfg, err := NewFromBytes(data, FormatYAML)
	require.NoError(t, err)

	_, err = Watch(cfg, func(c Config, err error) {})
	assert.ErrorIs(t, err, ErrNotFromFile)
}

func TestWatch_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	w, err := Watch(cfg, func(c Config, err error) {})
	require.NoError(t, err)

	w.StartAsync()

	// 停止监视
	err = w.Stop()
	assert.NoError(t, err)

	// 再次停止应该也是成功的（幂等）
	err = w.Stop()
	assert.NoError(t, err)
}

func TestWatch_WithDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	var mu sync.Mutex
	var reloadCount int

	// 使用较短的防抖时间
	w, err := Watch(cfg, func(c Config, err error) {
		mu.Lock()
		defer mu.Unlock()
		reloadCount++
	}, WithDebounce(50*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)

	// 快速连续修改多次
	for i := range 5 {
		content := []byte("app:\n  name: test" + string(rune('0'+i)) + "\n")
		err = os.WriteFile(configPath, content, 0600)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// 等待防抖完成
	time.Sleep(150 * time.Millisecond)

	// 由于防抖，回调次数应该少于修改次数
	mu.Lock()
	count := reloadCount
	mu.Unlock()
	assert.Less(t, count, 5, "debounce should reduce callback count")
}

func TestWatch_NilCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	_, err = Watch(cfg, nil)
	assert.ErrorIs(t, err, ErrNilCallback)
}

func TestWatch_InvalidDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	// 零值防抖
	_, err = Watch(cfg, func(c Config, err error) {}, WithDebounce(0))
	assert.ErrorIs(t, err, ErrInvalidDebounce)

	// 负值防抖
	_, err = Watch(cfg, func(c Config, err error) {}, WithDebounce(-time.Second))
	assert.ErrorIs(t, err, ErrInvalidDebounce)
}

func TestWatchConfig_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	// 验证 koanfConfig 实现了 WatchConfig 接口
	watchCfg, ok := cfg.(WatchConfig)
	require.True(t, ok, "koanfConfig should implement WatchConfig")

	// 通过接口创建监视器
	w, err := watchCfg.Watch(func(c Config, err error) {})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()
}

// =============================================================================
// 并发安全测试（针对修复的问题）
// =============================================================================

// TestWatcher_StopCancelsTimer 验证 Stop() 正确取消 debounce 定时器
// 修复问题：Stop() 后定时器可能仍然触发回调
func TestWatcher_StopCancelsTimer(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	var mu sync.Mutex
	callbackCalledAfterStop := false

	// 使用较长的防抖时间，以便有足够时间在回调前调用 Stop
	w, err := Watch(cfg, func(c Config, err error) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalledAfterStop = true
	}, WithDebounce(200*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)

	// 触发文件变更
	err = os.WriteFile(configPath, []byte("app:\n  name: updated\n"), 0600)
	require.NoError(t, err)

	// 等待事件被检测到，但在防抖回调触发前
	time.Sleep(50 * time.Millisecond)

	// 立即停止 - 这应该取消待执行的定时器
	err = w.Stop()
	require.NoError(t, err)

	// 等待足够长的时间，确保如果定时器没被取消，回调会被执行
	time.Sleep(300 * time.Millisecond)

	// 验证回调没有被调用（因为 Stop 取消了定时器）
	mu.Lock()
	called := callbackCalledAfterStop
	mu.Unlock()
	assert.False(t, called, "Stop() 后不应触发回调")
}

// TestWatcher_StartAsyncStopRace 验证 StartAsync/Stop 没有竞态
// 修复问题：StartAsync() 返回后立即调用 Stop() 可能因 running=false 而提前返回
func TestWatcher_StartAsyncStopRace(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	// 多次测试以增加暴露竞态的机会
	for range 100 {
		// 创建 watcher
		w, err := Watch(cfg, func(c Config, err error) {})
		require.NoError(t, err)

		// StartAsync 后立即 Stop
		w.StartAsync()
		err = w.Stop()
		// Stop 不应该返回错误（即使立即调用）
		assert.NoError(t, err, "Stop() 应该正常工作，即使在 StartAsync() 后立即调用")
	}
}

// TestWatcher_RenameEvent 验证 Rename 事件能触发配置重载
// 修复问题：vim/emacs 原子写入模式使用 Rename 而非 Write
func TestWatcher_RenameEvent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	var mu sync.Mutex
	var reloadCount int

	w, err := Watch(cfg, func(c Config, err error) {
		mu.Lock()
		defer mu.Unlock()
		reloadCount++
	}, WithDebounce(50*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)

	// 模拟原子写入：先写临时文件，然后 rename
	tmpFile := configPath + ".tmp"
	err = os.WriteFile(tmpFile, []byte("app:\n  name: renamed\n"), 0600)
	require.NoError(t, err)

	err = os.Rename(tmpFile, configPath)
	require.NoError(t, err)

	// 等待重载
	time.Sleep(200 * time.Millisecond)

	// 验证回调被调用
	mu.Lock()
	count := reloadCount
	mu.Unlock()
	assert.GreaterOrEqual(t, count, 1, "Rename 事件应触发回调")

	// 验证配置已更新
	assert.Equal(t, "renamed", cfg.Client().String("app.name"))
}

// =============================================================================
// 覆盖率补全测试
// =============================================================================

// TestWatch_EmptyPath 验证空路径时返回 ErrEmptyPath
func TestWatch_EmptyPath(t *testing.T) {
	// 手工构造一个 path 为空的 koanfConfig
	cfg := &koanfConfig{path: ""}
	_, err := Watch(cfg, func(c Config, err error) {})
	assert.ErrorIs(t, err, ErrEmptyPath)
}

// TestWatch_UnsupportedConfigType 验证非 koanfConfig 类型
func TestWatch_UnsupportedConfigType(t *testing.T) {
	// 传入 nil 接口
	_, err := Watch(nil, func(c Config, err error) {})
	assert.ErrorIs(t, err, ErrWatchFailed)
}

// TestWatcher_StartBlocking 验证 Start() 的阻塞行为
func TestWatcher_StartBlocking(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	w, err := Watch(cfg, func(c Config, err error) {})
	require.NoError(t, err)

	// 在 goroutine 中调用 Start，验证其阻塞直到 Stop
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		w.Start()
		close(done)
	}()

	<-started
	time.Sleep(20 * time.Millisecond)

	// Stop 应解除 Start 的阻塞
	err = w.Stop()
	require.NoError(t, err)

	select {
	case <-done:
		// Start 已返回 — 正常
	case <-time.After(time.Second):
		t.Fatal("Start() 未在 Stop() 后返回")
	}
}

// TestWatcher_DoubleStartAsync 验证重复调用 StartAsync 只启动一次
func TestWatcher_DoubleStartAsync(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	w, err := Watch(cfg, func(c Config, err error) {})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	w.StartAsync()
	// 第二次调用应直接返回（覆盖 running=true 分支）
	w.StartAsync()
}

// TestWatcher_DoubleStart 验证重复调用 Start 只启动一次
func TestWatcher_DoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	w, err := Watch(cfg, func(c Config, err error) {})
	require.NoError(t, err)

	// 先用 StartAsync 设置 running=true
	w.StartAsync()
	defer func() { _ = w.Stop() }()

	// 第二次调用 Start 应立即返回（覆盖 running=true 分支）
	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()

	select {
	case <-done:
		// 正常：Start 因 running=true 直接返回
	case <-time.After(time.Second):
		t.Fatal("Start() 应立即返回（已在运行）")
	}
}

// TestWatcher_CallbackPanic 验证用户回调 panic 不崩溃进程
func TestWatcher_CallbackPanic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	callbackCalled := make(chan struct{}, 1)

	// 回调故意 panic
	w, err := Watch(cfg, func(c Config, err error) {
		select {
		case callbackCalled <- struct{}{}:
		default:
		}
		panic("intentional panic in callback")
	}, WithDebounce(20*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)

	// 触发文件变更
	err = os.WriteFile(configPath, []byte("app:\n  name: updated\n"), 0600)
	require.NoError(t, err)

	// 等待回调被调用
	select {
	case <-callbackCalled:
		// 回调被调用且 panic 被恢复 — 正常
	case <-time.After(time.Second):
		t.Fatal("回调未被调用")
	}

	// 进程没有崩溃即验证通过
	time.Sleep(50 * time.Millisecond)
}

// TestWatcher_StopWithoutStart 验证未启动的 Watcher 调用 Stop 也能释放 fsnotify 资源
func TestWatcher_StopWithoutStart(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	w, err := Watch(cfg, func(c Config, err error) {})
	require.NoError(t, err)

	// 不调用 Start/StartAsync，直接 Stop 应释放 fsnotify 资源
	err = w.Stop()
	assert.NoError(t, err)

	// 再次 Stop 应幂等返回 nil
	err = w.Stop()
	assert.NoError(t, err)
}

// TestWatcher_HandleError 验证 fsnotify 错误通过回调传递
func TestWatcher_HandleError(t *testing.T) {
	// 直接测试 handleError 方法
	errCh := make(chan error, 1)
	w := &Watcher{
		cfg: &koanfConfig{},
		callback: func(c Config, err error) {
			errCh <- err
		},
	}

	testErr := fmt.Errorf("test fsnotify error")
	w.handleError(testErr)

	select {
	case err := <-errCh:
		assert.Contains(t, err.Error(), "watch error")
		assert.ErrorIs(t, err, testErr)
	case <-time.After(time.Second):
		t.Fatal("handleError 回调未被调用")
	}
}

// TestWatcher_HandleErrorNilCallback 验证无回调时 handleError 不 panic
func TestWatcher_HandleErrorNilCallback(t *testing.T) {
	w := &Watcher{
		cfg:      &koanfConfig{},
		callback: nil,
	}

	// 不应 panic
	assert.NotPanics(t, func() {
		w.handleError(fmt.Errorf("test error"))
	})
}
