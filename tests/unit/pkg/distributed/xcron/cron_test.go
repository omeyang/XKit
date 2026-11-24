package xcron

import (
	"context"
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

	t.Run("distributed locker without name returns ErrMissingName", func(t *testing.T) {
		locker := newMockLocker()
		s := New(WithLocker(locker))
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		})
		assert.ErrorIs(t, err, ErrMissingName)
	})

	t.Run("noop locker without name succeeds", func(t *testing.T) {
		s := New() // default NoopLocker
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("distributed locker with name succeeds", func(t *testing.T) {
		locker := newMockLocker()
		s := New(WithLocker(locker))
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("named-job"))
		assert.NoError(t, err)
	})

	checkScheduledJobNames(t)
}

func checkScheduledJobNames(t *testing.T) {
	t.Helper()
	t.Run("duplicate name returns ErrDuplicateJobName", func(t *testing.T) {
		s := New()
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("dup-job"))
		require.NoError(t, err)

		_, err = s.AddFunc("@every 2s", func(ctx context.Context) error {
			return nil
		}, WithName("dup-job"))
		assert.ErrorIs(t, err, ErrDuplicateJobName)
	})

	t.Run("different names succeed", func(t *testing.T) {
		s := New()
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("job-a"))
		require.NoError(t, err)

		_, err = s.AddFunc("@every 2s", func(ctx context.Context) error {
			return nil
		}, WithName("job-b"))
		assert.NoError(t, err)
	})

	t.Run("removed name can be reused", func(t *testing.T) {
		s := New()
		defer s.Stop()

		id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("reuse-job"))
		require.NoError(t, err)

		s.Remove(id)

		_, err = s.AddFunc("@every 2s", func(ctx context.Context) error {
			return nil
		}, WithName("reuse-job"))
		assert.NoError(t, err)
	})

	t.Run("name reusable after Cron().Remove() bypasses xcron.Remove()", func(t *testing.T) {
		s := New()
		defer s.Stop()

		id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		}, WithName("bypass-job"))
		require.NoError(t, err)

		// 通过 Cron().Remove() 直接移除，绕过 xcron.Remove()
		s.Cron().Remove(id)

		// 自修复：validateJobName 检测到 cron 中已无此条目，自动清理 jobNames
		_, err = s.AddFunc("@every 2s", func(ctx context.Context) error {
			return nil
		}, WithName("bypass-job"))
		assert.NoError(t, err)
	})

	t.Run("unnamed jobs with noop locker are not checked for duplicates", func(t *testing.T) {
		s := New()
		defer s.Stop()

		_, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
			return nil
		})
		require.NoError(t, err)

		// 无名任务不参与唯一性校验
		_, err = s.AddFunc("@every 2s", func(ctx context.Context) error {
			return nil
		})
		assert.NoError(t, err)
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

func TestScheduler_AddJob_TypedNilJobFunc(t *testing.T) {
	s := New()
	defer s.Stop()

	_, err := s.AddJob("@every 1s", JobFunc(nil))
	assert.ErrorIs(t, err, ErrNilJob)
}

func TestScheduler_AddFunc_TypedNilJobFunc(t *testing.T) {
	s := New()
	defer s.Stop()

	_, err := s.AddFunc("@every 1s", nil)
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

// panicLocker 用于测试 TryLock panic 隔离
type panicLocker struct{}

func (l *panicLocker) TryLock(_ context.Context, _ string, _ time.Duration) (LockHandle, error) {
	panic("locker panic in TryLock")
}

var _ Locker = (*panicLocker)(nil)

// panicRenewHandle 用于测试 Renew panic 隔离
type panicRenewHandle struct {
	key string
}

func (h *panicRenewHandle) Unlock(_ context.Context) error { return nil }
func (h *panicRenewHandle) Renew(_ context.Context, _ time.Duration) error {
	panic("locker panic in Renew")
}
func (h *panicRenewHandle) Key() string { return h.key }

// panicRenewLocker 正常获取锁，但 Renew 会 panic
type panicRenewLocker struct{}

func (l *panicRenewLocker) TryLock(_ context.Context, key string, _ time.Duration) (LockHandle, error) {
	return &panicRenewHandle{key: key}, nil
}

var _ Locker = (*panicRenewLocker)(nil)

// panicObserver 用于测试 tracer.Start panic 隔离
type panicObserver struct{}

func (o *panicObserver) Start(_ context.Context, _ string, _ ...any) (context.Context, Span) {
	panic("observer panic in Start")
}

var _ Observer = (*panicObserver)(nil)
