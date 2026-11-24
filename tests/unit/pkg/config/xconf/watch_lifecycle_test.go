package xconf

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWatcher_CallbackPanicWithLoggerPanic 验证 slog handler 也 panic 时嵌套 recover 防护生效。
// 回归测试：safeCallback 的 recover 内调用 slog.Error 记录日志，若外部 slog handler
// 实现本身 panic（如格式化 Attr 时），嵌套 defer recover 确保回调 goroutine 不崩溃。
func TestWatcher_CallbackPanicWithLoggerPanic(t *testing.T) {
	old := slog.Default()
	slog.SetDefault(slog.New(panicHandler{}))
	defer slog.SetDefault(old)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	callbackCalled := make(chan struct{}, 1)

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

	err = os.WriteFile(configPath, []byte("app:\n  name: updated\n"), 0600)
	require.NoError(t, err)

	select {
	case <-callbackCalled:
	case <-time.After(time.Second):
		t.Fatal("回调未被调用")
	}

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

// TestWatcher_HandleError 验证 fsnotify 错误通过回调传递（包含文件路径）
func TestWatcher_HandleError(t *testing.T) {
	// 直接测试 handleError 方法
	errCh := make(chan error, 1)
	w := &Watcher{
		cfg:     &koanfConfig{path: "/etc/app/config.yaml"},
		running: true,
		callback: func(c Config, err error) {
			errCh <- err
		},
	}

	testErr := fmt.Errorf("test fsnotify error")
	w.handleError(testErr)

	select {
	case err := <-errCh:
		assert.Contains(t, err.Error(), "watch error")
		assert.Contains(t, err.Error(), "/etc/app/config.yaml")
		assert.ErrorIs(t, err, testErr)
	case <-time.After(time.Second):
		t.Fatal("handleError 回调未被调用")
	}
}

// TestWatcher_HandleErrorNotRunning 验证 running=false 时 handleError 不调用回调
func TestWatcher_HandleErrorNotRunning(t *testing.T) {
	callbackCalled := false
	w := &Watcher{
		cfg:     &koanfConfig{path: "/etc/app/config.yaml"},
		running: false,
		callback: func(c Config, err error) {
			callbackCalled = true
		},
	}

	w.handleError(fmt.Errorf("test error"))
	assert.False(t, callbackCalled, "handleError 在 running=false 时不应调用回调")
}

// TestWatch_DirectoryDeletedBeforeWatch 验证 Watch 在目录不存在时返回 ErrWatchFailed
// 覆盖 fsnotify.Add 失败 + errors.Join 路径
func TestWatch_DirectoryDeletedBeforeWatch(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	// 先通过 New 加载（此时目录存在）
	cfg, err := New(configPath)
	require.NoError(t, err)

	// 删除目录，使 fsnotify.Add 失败
	err = os.RemoveAll(tmpDir)
	require.NoError(t, err)

	_, err = Watch(cfg, func(c Config, err error) {})
	assert.ErrorIs(t, err, ErrWatchFailed)
	assert.Contains(t, err.Error(), "failed to watch directory")
}

// TestWatcher_HandleErrorNilCallback 验证无回调时 handleError 不 panic
func TestWatcher_HandleErrorNilCallback(t *testing.T) {
	w := &Watcher{
		cfg:      &koanfConfig{},
		running:  true,
		callback: nil,
	}

	// 不应 panic
	assert.NotPanics(t, func() {
		w.handleError(fmt.Errorf("test error"))
	})
}

// TestWatcher_StopFromCallback 验证在回调内调用 Stop() 不会死锁
// 修复问题 FG-S1：防抖回调 goroutine 先 callbackWg.Add(1)，若用户在回调中
// 调用 Stop()，Stop() 会 callbackWg.Wait() 等待当前回调结束，形成自锁死锁。
func TestWatcher_StopFromCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	stopDone := make(chan error, 1)

	w, err := Watch(cfg, func(c Config, cbErr error) {
		// 在回调中调用 Stop() — 修复前会死锁
		// 这里使用闭包引用外层 w，因为回调在 Watch 返回后才会执行
	}, WithDebounce(20*time.Millisecond))
	require.NoError(t, err)

	// 重新设置回调（需要引用 w）
	w.callback = func(c Config, cbErr error) {
		stopDone <- w.Stop()
	}

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)

	// 触发文件变更
	err = os.WriteFile(configPath, []byte("app:\n  name: updated\n"), 0600)
	require.NoError(t, err)

	// 等待 Stop 完成（设超时防止死锁导致测试挂起）
	select {
	case stopErr := <-stopDone:
		assert.NoError(t, stopErr, "Stop() 在回调内调用不应返回错误")
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() 在回调内调用导致死锁（超时 3 秒）")
	}
}

// TestWatcher_StopFromErrorCallback 验证在错误回调内调用 Stop() 不会死锁。
// 回归测试 FG-S1：原实现中 handleError 在 run() goroutine 内直接调用 safeCallback，
// 回调内调用 Stop() 会因 runWg.Wait() 等待 run() 自身退出而死锁。
// 修复方案：handleError 通过 dispatchCallback 在独立 goroutine 中执行回调，
// run() 不直接执行用户回调，runWg.Wait() 不会死锁。
func TestWatcher_StopFromErrorCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("app:\n  name: test\n"), 0600)
	require.NoError(t, err)

	cfg, err := New(configPath)
	require.NoError(t, err)

	stopDone := make(chan error, 1)

	w, err := Watch(cfg, func(c Config, cbErr error) {
		// 占位回调
	})
	require.NoError(t, err)

	// 重新设置回调：在错误回调中调用 Stop()
	w.callback = func(c Config, cbErr error) {
		if cbErr != nil {
			stopDone <- w.Stop()
		}
	}

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)

	// 模拟 fsnotify 错误：handleError 在 run() goroutine 中被调用
	// 由于无法直接注入 fsnotify 错误通道，我们通过删除监视目录触发错误
	// 注意：此测试同时验证了 handleError 的 running 检查
	err = os.RemoveAll(tmpDir)
	require.NoError(t, err)

	// 等待 Stop 完成或超时
	select {
	case stopErr := <-stopDone:
		assert.NoError(t, stopErr, "Stop() 在错误回调内调用不应返回错误")
	case <-time.After(3 * time.Second):
		// fsnotify 可能未产生错误（平台差异），正常停止
		err = w.Stop()
		assert.NoError(t, err)
	}
}

// TestGoid 验证 goid() 返回有效的 goroutine ID
func TestGoid(t *testing.T) {
	id := goid()
	assert.Greater(t, id, int64(0), "goid() 应返回正整数")

	// 不同 goroutine 应有不同 ID
	ch := make(chan int64, 1)
	go func() {
		ch <- goid()
	}()
	otherId := <-ch
	assert.Greater(t, otherId, int64(0))
	assert.NotEqual(t, id, otherId, "不同 goroutine 应有不同 ID")
}

// TestWatcher_ZeroValueStop 验证零值 Watcher 调用 Stop 返回错误而非 panic。
func TestWatcher_ZeroValueStop(t *testing.T) {
	var w Watcher
	err := w.Stop()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWatchFailed)
	assert.Contains(t, err.Error(), "not initialized")
}

// TestWatch_TypedNilConfig 验证 typed nil Config 不会 panic
func TestWatch_TypedNilConfig(t *testing.T) {
	var kc *koanfConfig
	var cfg Config = kc
	_, err := Watch(cfg, func(c Config, err error) {})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWatchFailed)
}

// TestWatcher_StartAfterStopNoCallback 验证 Stop() 后再次调用 Start/StartAsync 不会触发回调
func TestWatcher_StartAfterStopNoCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("k: v\n"), 0600))

	cfg, err := New(configPath)
	require.NoError(t, err)

	var callCount int
	var mu sync.Mutex
	kc, ok := cfg.(*koanfConfig)
	require.True(t, ok)
	w, err := Watch(kc, func(c Config, err error) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})
	require.NoError(t, err)

	require.NoError(t, w.Stop())

	// Stop 后再次 StartAsync：应为 no-op，不应触发 "channel closed" 回调
	w.StartAsync()
	// Start 同样应为 no-op
	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start() after Stop() 应立即返回")
	}

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, callCount, "Stop() 后 Start/StartAsync 不应触发回调")
}

// TestWatcher_ConcurrentStopIdempotent 验证并发 Stop 共享同一完成点（A1 修复）。
// 修复前第二个 Stop 立即返回 nil，但第一个 Stop 仍在 Wait，回调可能仍在执行。
// 修复后非首个调用方等 stopDone 再返回，保证契约："Stop 返回后不再有回调执行"。
func TestWatcher_ConcurrentStopIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("k: v\n"), 0600))

	cfg, err := New(configPath)
	require.NoError(t, err)

	// 长耗时回调：放大并发 Stop 窗口
	var callbackActive atomic.Int32
	w, err := Watch(cfg, func(c Config, cbErr error) {
		callbackActive.Store(1)
		time.Sleep(100 * time.Millisecond)
		callbackActive.Store(0)
	}, WithDebounce(20*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, os.WriteFile(configPath, []byte("k: v2\n"), 0600))
	time.Sleep(40 * time.Millisecond) // 进入回调

	// 并发两次 Stop，要求两者返回时均满足契约
	var wg sync.WaitGroup
	stopResults := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stopResults[idx] = w.Stop()
			// 返回时回调必须已结束
			assert.Equal(t, int32(0), callbackActive.Load(),
				"Stop() 返回时不应有回调仍在执行（A1 修复）")
		}(i)
	}
	wg.Wait()

	for _, err := range stopResults {
		assert.NoError(t, err)
	}
}

// TestWatcher_StartAsyncStopNoSpuriousCallback 验证 StartAsync/Stop 瞬时切换
// 不会触发"channel closed"回调（A2 修复）。
// 修复前 runWg.Add(1) 在 mu 解锁后执行，Stop 可能在 Add 前 Wait 完成并 Close，
// 随后 run goroutine 启动并在已关闭 watcher 上读到 ok=false 触发回调。
func TestWatcher_StartAsyncStopNoSpuriousCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("k: v\n"), 0600))

	cfg, err := New(configPath)
	require.NoError(t, err)

	var callbackCount atomic.Int32
	for range 200 {
		w, err := Watch(cfg, func(c Config, cbErr error) {
			callbackCount.Add(1)
		})
		require.NoError(t, err)

		w.StartAsync()
		require.NoError(t, w.Stop())
	}
	// Stop 立即调用后不应触发任何"unexpected channel closed"回调
	assert.Equal(t, int32(0), callbackCount.Load(),
		"StartAsync+Stop 立即调用不应触发虚假回调（A2 修复）")
}

// TestWatcher_StopWaitsForOtherCallbacks 验证回调内调用 Stop 会等待
// 其他并发 in-flight 回调完成（A3 修复）。
// 修复前 Stop 直接跳过整个 callbackWg.Wait()，不等待并发回调；
// 修复后用 per-callback done channel，只跳过自身、等待其他。
func TestWatcher_StopWaitsForOtherCallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("k: v\n"), 0600))

	cfg, err := New(configPath)
	require.NoError(t, err)

	var w *Watcher
	otherRunning := make(chan struct{})
	otherDone := make(chan struct{})
	stopReturned := make(chan error, 1)
	isFirst := true
	var mu sync.Mutex

	w, err = Watch(cfg, func(c Config, cbErr error) {
		mu.Lock()
		first := isFirst
		isFirst = false
		mu.Unlock()
		if first {
			// 第一个回调：慢回调，模拟并发 in-flight
			close(otherRunning)
			time.Sleep(150 * time.Millisecond)
			close(otherDone)
			return
		}
		// 第二个回调：在回调内 Stop，要求等待第一个完成
		stopReturned <- w.Stop()
	}, WithDebounce(20*time.Millisecond))
	require.NoError(t, err)

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)

	// 触发第一个回调（慢）
	require.NoError(t, os.WriteFile(configPath, []byte("k: v2\n"), 0600))
	<-otherRunning

	// 第一个回调仍在跑时，触发第二个回调（调用 Stop）
	require.NoError(t, os.WriteFile(configPath, []byte("k: v3\n"), 0600))

	select {
	case err := <-stopReturned:
		require.NoError(t, err)
		// Stop 返回时第一个回调必须已完成
		select {
		case <-otherDone:
			// 正常
		default:
			t.Fatal("Stop() 返回时其他并发回调未结束（A3 修复验证失败）")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() 超时（可能死锁）")
	}
}

// TestWatcher_StopSuppressesChannelClosed 验证外部 Stop 后 run() 不再触发
// "channel closed unexpectedly"回调（A4 修复）。
func TestWatcher_StopSuppressesChannelClosed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("k: v\n"), 0600))

	cfg, err := New(configPath)
	require.NoError(t, err)

	var unexpectedCount atomic.Int32
	w, err := Watch(cfg, func(c Config, cbErr error) {
		if cbErr != nil && strings.Contains(cbErr.Error(), "channel closed unexpectedly") {
			unexpectedCount.Add(1)
		}
	})
	require.NoError(t, err)

	w.StartAsync()
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, w.Stop())
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(0), unexpectedCount.Load(),
		"Stop 后不应触发 channel closed unexpectedly 回调（A4 修复）")
}

// TestWatcher_K8sConfigMapSymlink 验证接收 ..data symlink 事件触发 Reload（B3 修复）。
// K8s ConfigMap/Secret 挂载使用 atomic symlink 模式，kubelet 更新时只 rename ..data。
// 修复前 basename 过滤丢弃 ..data 事件，热更新失效；修复后接受 ..data。
func TestWatcher_K8sConfigMapSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink requires admin privileges on Windows")
	}
	tmpDir := t.TempDir()
	// 模拟 K8s 布局：实际数据在 ..data 子目录（通过 symlink 指向），
	// config.yaml 是 ..data/config.yaml 的 symlink
	dataDir := filepath.Join(tmpDir, "..2026_04_17")
	require.NoError(t, os.Mkdir(dataDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "config.yaml"), []byte("k: v\n"), 0600))
	require.NoError(t, os.Symlink("..2026_04_17", filepath.Join(tmpDir, "..data")))
	require.NoError(t, os.Symlink("..data/config.yaml", filepath.Join(tmpDir, "config.yaml")))

	cfg, err := New(filepath.Join(tmpDir, "config.yaml"))
	require.NoError(t, err)

	reloaded := make(chan struct{}, 1)
	w, err := Watch(cfg, func(c Config, cbErr error) {
		select {
		case reloaded <- struct{}{}:
		default:
		}
	}, WithDebounce(20*time.Millisecond))
	require.NoError(t, err)
	w.StartAsync()
	defer func() { _ = w.Stop() }()
	time.Sleep(30 * time.Millisecond)

	// 模拟 kubelet 更新：创建新 data 目录 + 原子 rename ..data symlink
	newDataDir := filepath.Join(tmpDir, "..2026_04_17b")
	require.NoError(t, os.Mkdir(newDataDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(newDataDir, "config.yaml"), []byte("k: v2\n"), 0600))
	// fsnotify 不原生支持 symlink 原子 rename，退而求其次用删除再创建
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "..data")))
	require.NoError(t, os.Symlink("..2026_04_17b", filepath.Join(tmpDir, "..data")))

	select {
	case <-reloaded:
		// ..data 事件触发了回调（B3 修复）
	case <-time.After(time.Second):
		t.Fatal("..data symlink 事件未触发回调（B3 修复验证失败）")
	}
}
