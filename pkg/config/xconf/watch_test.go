package xconf

import (
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
	defer func() { _ = w.Stop() }() //nolint:errcheck // 测试清理

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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot watch config created from bytes")
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
	defer func() { _ = w.Stop() }() //nolint:errcheck // 测试清理

	time.Sleep(30 * time.Millisecond)

	// 快速连续修改多次
	for i := 0; i < 5; i++ {
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
	defer func() { _ = w.Stop() }() //nolint:errcheck // 测试清理
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
	for i := 0; i < 100; i++ {
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
	defer func() { _ = w.Stop() }() //nolint:errcheck // 测试清理

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
