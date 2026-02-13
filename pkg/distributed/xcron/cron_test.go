package xcron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		s := New()
		require.NotNil(t, s)
		assert.NotNil(t, s.Cron())
	})

	t.Run("with locker", func(t *testing.T) {
		locker := NoopLocker()
		s := New(WithLocker(locker))
		require.NotNil(t, s)
	})

	t.Run("with seconds", func(t *testing.T) {
		s := New(WithSeconds())
		require.NotNil(t, s)

		// 验证可以解析秒级表达式
		_, err := s.AddFunc("*/5 * * * * *", func(ctx context.Context) error {
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("with location", func(t *testing.T) {
		loc, err := time.LoadLocation("Asia/Shanghai")
		require.NoError(t, err)
		s := New(WithLocation(loc))
		require.NotNil(t, s)
	})
}

func TestScheduler_AddFunc(t *testing.T) {
	t.Run("valid cron expression", func(t *testing.T) {
		s := New()
		defer s.Stop()

		id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		})
		assert.NoError(t, err)
		assert.NotZero(t, id)
	})

	t.Run("invalid cron expression", func(t *testing.T) {
		s := New()
		defer s.Stop()

		_, err := s.AddFunc("invalid", func(ctx context.Context) error {
			return nil
		})
		assert.Error(t, err)
	})

	t.Run("with name option", func(t *testing.T) {
		s := New()
		defer s.Stop()

		id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("test-job"))
		assert.NoError(t, err)
		assert.NotZero(t, id)
	})

	t.Run("nil cmd returns ErrNilJob", func(t *testing.T) {
		s := New()
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", nil)
		assert.ErrorIs(t, err, ErrNilJob)
	})
}

func TestScheduler_AddJob(t *testing.T) {
	s := New()
	defer s.Stop()

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	id, err := s.AddJob("@every 1s", job, WithName("test-job"))
	assert.NoError(t, err)
	assert.NotZero(t, id)
}

func TestScheduler_AddJob_NilJob(t *testing.T) {
	s := New()
	defer s.Stop()

	_, err := s.AddJob("@every 1s", nil)
	assert.ErrorIs(t, err, ErrNilJob)
}

func TestScheduler_Remove(t *testing.T) {
	s := New()
	defer s.Stop()

	id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)

	// 验证任务存在
	entries := s.Entries()
	assert.Len(t, entries, 1)

	// 移除任务
	s.Remove(id)

	// 验证任务已移除
	entries = s.Entries()
	assert.Len(t, entries, 0)
}

func TestScheduler_StartStop(t *testing.T) {
	s := New()

	var counter atomic.Int32

	_, err := s.AddFunc("@every 100ms", func(ctx context.Context) error {
		counter.Add(1)
		return nil
	})
	require.NoError(t, err)

	// 启动调度器
	s.Start()

	// 等待足够时间让任务执行多次（cron 任务对齐到秒边界，需要更长等待时间）
	time.Sleep(1200 * time.Millisecond)

	// 停止调度器
	ctx := s.Stop()
	<-ctx.Done()

	// 验证任务至少执行了一次
	count := counter.Load()
	assert.GreaterOrEqual(t, count, int32(1))
	assert.LessOrEqual(t, count, int32(15))
}

func TestScheduler_Entries(t *testing.T) {
	s := New()
	defer s.Stop()

	// 添加多个任务
	_, err := s.AddFunc("@every 1s", func(ctx context.Context) error { return nil })
	require.NoError(t, err)
	_, err = s.AddFunc("@every 2s", func(ctx context.Context) error { return nil })
	require.NoError(t, err)
	_, err = s.AddFunc("@every 3s", func(ctx context.Context) error { return nil })
	require.NoError(t, err)

	entries := s.Entries()
	assert.Len(t, entries, 3)
}

func TestScheduler_Cron(t *testing.T) {
	s := New()
	defer s.Stop()

	c := s.Cron()
	require.NotNil(t, c)

	// 可以使用原生 API
	id := c.Schedule(nil, nil)
	_ = id // 验证可以调用原生方法
}

func TestJobFunc(t *testing.T) {
	var called bool

	job := JobFunc(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := job.Run(context.Background())
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestJobWrapper_Run(t *testing.T) {
	t.Run("basic execution", func(t *testing.T) {
		var executed bool

		job := JobFunc(func(ctx context.Context) error {
			executed = true
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
		wrapper.Run()

		assert.True(t, executed)
	})

	t.Run("with timeout", func(t *testing.T) {
		var receivedCtx context.Context

		job := JobFunc(func(ctx context.Context) error {
			receivedCtx = ctx
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.timeout = 5 * time.Second
		wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
		wrapper.Run()

		// 验证 context 有 deadline
		_, ok := receivedCtx.Deadline()
		assert.True(t, ok)
	})

	t.Run("with noop locker", func(t *testing.T) {
		var executed bool

		job := JobFunc(func(ctx context.Context) error {
			executed = true
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
		wrapper.Run()

		assert.True(t, executed)
	})
}

func TestJobWrapper_PanicRecovery(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		job := JobFunc(func(ctx context.Context) error {
			panic("test panic")
		})

		stats := newStats()
		opts := defaultJobOptions()
		opts.name = "panic-job"
		wrapper := newJobWrapper(job, NoopLocker(), nil, stats, opts)

		// 不应 panic
		require.NotPanics(t, func() {
			wrapper.Run()
		})

		// panic 应被记录为失败
		assert.Equal(t, int64(1), stats.TotalExecutions())
		assert.Equal(t, int64(1), stats.FailureCount())
		assert.Contains(t, stats.LastError().Error(), "panicked")
	})

	t.Run("recovers from panic with hooks", func(t *testing.T) {
		var afterCalled bool
		var afterErr error
		hook := HookFunc{
			After: func(ctx context.Context, name string, d time.Duration, err error) {
				afterCalled = true
				afterErr = err
			},
		}

		job := JobFunc(func(ctx context.Context) error {
			panic("hook panic test")
		})

		opts := defaultJobOptions()
		opts.name = "panic-hook-job"
		opts.hooks = []Hook{hook}
		wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)

		require.NotPanics(t, func() {
			wrapper.Run()
		})

		// AfterJob 应被调用，且 err 应包含 panic 信息
		assert.True(t, afterCalled)
		assert.Contains(t, afterErr.Error(), "panicked")
	})
}

func TestJobWrapper_ConcurrentExecution(t *testing.T) {
	// 测试多个 wrapper 并发执行时，使用锁的情况
	locker := newMockLocker()

	var mu sync.Mutex
	var executions []int

	createJob := func(id int) Job {
		return JobFunc(func(ctx context.Context) error {
			mu.Lock()
			executions = append(executions, id)
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			return nil
		})
	}

	opts1 := defaultJobOptions()
	opts1.name = "shared-job"

	opts2 := defaultJobOptions()
	opts2.name = "shared-job"

	wrapper1 := newJobWrapper(createJob(1), locker, nil, nil, opts1)
	wrapper2 := newJobWrapper(createJob(2), locker, nil, nil, opts2)

	// 并发执行
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		wrapper1.Run()
	}()
	go func() {
		defer wg.Done()
		wrapper2.Run()
	}()
	wg.Wait()

	// 验证只有一个获得了锁并执行
	mu.Lock()
	assert.Len(t, executions, 1, "Only one job should execute due to lock")
	mu.Unlock()
}

// mockLocker 用于测试的模拟锁
type mockLocker struct {
	mu    sync.Mutex
	locks map[string]string // key -> token
}

// mockLockHandle 模拟锁句柄
type mockLockHandle struct {
	locker *mockLocker
	key    string
	token  string
}

func newMockLocker() *mockLocker {
	return &mockLocker{
		locks: make(map[string]string),
	}
}

func (l *mockLocker) TryLock(_ context.Context, key string, _ time.Duration) (LockHandle, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, held := l.locks[key]; held {
		return nil, nil
	}
	token := "mock-token-" + key
	l.locks[key] = token
	return &mockLockHandle{locker: l, key: key, token: token}, nil
}

func (h *mockLockHandle) Unlock(_ context.Context) error {
	h.locker.mu.Lock()
	defer h.locker.mu.Unlock()
	if h.locker.locks[h.key] != h.token {
		return ErrLockNotHeld
	}
	delete(h.locker.locks, h.key)
	return nil
}

func (h *mockLockHandle) Renew(_ context.Context, _ time.Duration) error {
	h.locker.mu.Lock()
	defer h.locker.mu.Unlock()
	if h.locker.locks[h.key] != h.token {
		return ErrLockNotHeld
	}
	return nil
}

func (h *mockLockHandle) Key() string {
	return h.key
}

var _ Locker = (*mockLocker)(nil)
var _ LockHandle = (*mockLockHandle)(nil)

// ============================================================================
// WithImmediate Integration Tests
// ============================================================================

func TestScheduler_WithImmediate(t *testing.T) {
	t.Run("executes immediately after AddFunc", func(t *testing.T) {
		s := New()
		defer s.Stop()

		var counter atomic.Int32
		_, err := s.AddFunc("@every 1h", func(ctx context.Context) error {
			counter.Add(1)
			return nil
		}, WithName("immediate-job"), WithImmediate())

		require.NoError(t, err)

		// 等待立即执行完成（异步执行）
		time.Sleep(100 * time.Millisecond)

		// 验证任务已经执行了一次（立即执行）
		assert.Equal(t, int32(1), counter.Load())
	})

	t.Run("without immediate does not execute before Start", func(t *testing.T) {
		s := New()
		defer s.Stop()

		var counter atomic.Int32
		_, err := s.AddFunc("@every 1h", func(ctx context.Context) error {
			counter.Add(1)
			return nil
		}, WithName("normal-job"))

		require.NoError(t, err)

		// 等待一段时间
		time.Sleep(100 * time.Millisecond)

		// 验证任务没有执行
		assert.Equal(t, int32(0), counter.Load())
	})

	t.Run("immediate execution uses lock", func(t *testing.T) {
		locker := newMockLocker()
		s := New(WithLocker(locker))
		defer s.Stop()

		var executed atomic.Bool
		_, err := s.AddFunc("@every 1h", func(ctx context.Context) error {
			executed.Store(true)
			return nil
		}, WithName("locked-immediate-job"), WithImmediate())

		require.NoError(t, err)

		// 等待立即执行完成
		time.Sleep(100 * time.Millisecond)

		assert.True(t, executed.Load())
	})

	t.Run("immediate execution with timeout", func(t *testing.T) {
		s := New()
		defer s.Stop()

		var receivedCtx context.Context
		var executed atomic.Bool

		_, err := s.AddFunc("@every 1h", func(ctx context.Context) error {
			receivedCtx = ctx
			executed.Store(true)
			return nil
		}, WithName("timeout-immediate-job"), WithImmediate(), WithTimeout(5*time.Second))

		require.NoError(t, err)

		// 等待立即执行完成
		time.Sleep(100 * time.Millisecond)

		assert.True(t, executed.Load())
		// 验证 context 有 deadline（超时控制生效）
		_, hasDeadline := receivedCtx.Deadline()
		assert.True(t, hasDeadline)
	})

	t.Run("immediate plus scheduled execution", func(t *testing.T) {
		s := New(WithSeconds())
		defer s.Stop()

		var counter atomic.Int32
		_, err := s.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
			counter.Add(1)
			return nil
		}, WithName("immediate-scheduled-job"), WithImmediate())

		require.NoError(t, err)

		// 立即执行
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, int32(1), counter.Load())

		// 启动调度器
		s.Start()

		// 等待定时任务执行（秒级精度，等待足够时间）
		time.Sleep(1500 * time.Millisecond)

		// 验证至少执行了 2 次（立即执行 1 次 + 定时执行至少 1 次）
		assert.GreaterOrEqual(t, counter.Load(), int32(2))
	})

	t.Run("multiple jobs with immediate", func(t *testing.T) {
		s := New()
		defer s.Stop()

		var counter1, counter2 atomic.Int32

		_, err := s.AddFunc("@every 1h", func(ctx context.Context) error {
			counter1.Add(1)
			return nil
		}, WithName("job1"), WithImmediate())
		require.NoError(t, err)

		_, err = s.AddFunc("@every 1h", func(ctx context.Context) error {
			counter2.Add(1)
			return nil
		}, WithName("job2"), WithImmediate())
		require.NoError(t, err)

		// 等待立即执行完成
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, int32(1), counter1.Load())
		assert.Equal(t, int32(1), counter2.Load())
	})

	t.Run("immediate execution error does not prevent registration", func(t *testing.T) {
		s := New(WithSeconds())
		defer s.Stop()

		var attempts atomic.Int32
		_, err := s.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
			attempts.Add(1)
			if attempts.Load() == 1 {
				return assert.AnError // 第一次（立即执行）失败
			}
			return nil
		}, WithName("error-immediate-job"), WithImmediate())

		require.NoError(t, err)

		// 等待立即执行完成
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, int32(1), attempts.Load())

		// 启动调度器，验证任务仍然被正常调度
		s.Start()
		time.Sleep(1500 * time.Millisecond)

		// 任务应该继续执行（秒级调度，等待 1.5 秒应该至少执行 1 次）
		assert.GreaterOrEqual(t, attempts.Load(), int32(2))
	})
}

func TestScheduler_AddJob_WithImmediate(t *testing.T) {
	s := New()
	defer s.Stop()

	var executed atomic.Bool
	job := JobFunc(func(ctx context.Context) error {
		executed.Store(true)
		return nil
	})

	_, err := s.AddJob("@every 1h", job, WithName("immediate-job"), WithImmediate())
	require.NoError(t, err)

	// 等待立即执行完成
	time.Sleep(100 * time.Millisecond)

	assert.True(t, executed.Load())
}
