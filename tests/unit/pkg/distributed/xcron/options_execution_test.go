package xcron

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobWrapper_LogDebug(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)

		wrapper.logDebug(context.Background(), "test debug", "key", "value")

		assert.Equal(t, 1, logger.getDebugCount())
	})

	t.Run("without logger does nothing", func(t *testing.T) {
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), nil, nil, opts)

		// 不应该 panic 也不应该有输出
		wrapper.logDebug(context.Background(), "test debug", "key", "value")
	})
}

// ============================================================================
// Wrapper Retry Tests
// ============================================================================

func TestJobWrapper_RunWithRetry(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		var attempts int
		job := JobFunc(func(ctx context.Context) error {
			attempts++
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = newMockRetryPolicy(3)
		opts.backoff = newMockBackoffPolicy(1 * time.Millisecond)

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)
		err := wrapper.runWithRetry(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("retries on failure", func(t *testing.T) {
		var attempts int
		job := JobFunc(func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = newMockRetryPolicy(5)
		opts.backoff = newMockBackoffPolicy(1 * time.Millisecond)

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)
		err := wrapper.runWithRetry(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("returns error after max retries", func(t *testing.T) {
		var attempts int
		job := JobFunc(func(ctx context.Context) error {
			attempts++
			return errors.New("persistent error")
		})

		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = newMockRetryPolicy(3) // 只重试 2 次（attempt < 3）
		opts.backoff = newMockBackoffPolicy(1 * time.Millisecond)

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)
		err := wrapper.runWithRetry(context.Background())

		assert.Error(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		var attempts int
		job := JobFunc(func(ctx context.Context) error {
			attempts++
			return errors.New("error")
		})

		ctx, cancel := context.WithCancel(context.Background())
		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = newMockRetryPolicy(100)
		opts.backoff = newMockBackoffPolicy(100 * time.Millisecond)

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)

		// 在另一个 goroutine 中取消 context
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := wrapper.runWithRetry(ctx)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("works without backoff policy", func(t *testing.T) {
		var attempts int
		job := JobFunc(func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errors.New("error")
			}
			return nil
		})

		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = newMockRetryPolicy(3)
		opts.backoff = nil // 无退避策略

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)
		err := wrapper.runWithRetry(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})

	t.Run("exits on canceled ctx with zero backoff", func(t *testing.T) {
		var attempts atomic.Int32
		job := JobFunc(func(_ context.Context) error {
			attempts.Add(1)
			return errors.New("always fail")
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		opts := defaultJobOptions()
		opts.name = "retry-job"
		opts.retry = &alwaysRetryPolicy{}
		opts.backoff = nil // backoff=0

		wrapper := newJobWrapper(job, NoopLocker(), newMockLogger(), nil, opts)
		err := wrapper.runWithRetry(ctx)

		assert.ErrorIs(t, err, context.Canceled)
		assert.LessOrEqual(t, attempts.Load(), int32(1))
	})
}

// alwaysRetryPolicy 无限重试策略，用于测试忙循环防御
type alwaysRetryPolicy struct{}

func (p *alwaysRetryPolicy) ShouldRetry(_ int, _ error) bool { return true }

// ============================================================================
// Wrapper startSpan Tests
// ============================================================================

func TestJobWrapper_StartSpan(t *testing.T) {
	t.Run("with tracer", func(t *testing.T) {
		tracer := newMockObserver()
		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.tracer = tracer

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), nil, nil, opts)
		ctx, span := wrapper.startSpan(context.Background())

		assert.NotNil(t, ctx)
		assert.NotNil(t, span)
		assert.Equal(t, int32(1), tracer.spanStarted.Load())
	})

	t.Run("without tracer", func(t *testing.T) {
		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.tracer = nil

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), nil, nil, opts)
		ctx, span := wrapper.startSpan(context.Background())

		assert.NotNil(t, ctx)
		assert.Nil(t, span)
	})
}

// ============================================================================
// Wrapper logResult Tests
// ============================================================================

func TestJobWrapper_LogResult(t *testing.T) {
	t.Run("success with span", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)
		span := &mockSpan{}

		wrapper.logResult(context.Background(), span, 100*time.Millisecond, nil)

		assert.Equal(t, 1, logger.getDebugCount())
		assert.False(t, span.errorRecorded)
	})

	t.Run("error with span", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)
		span := &mockSpan{}

		wrapper.logResult(context.Background(), span, 100*time.Millisecond, errors.New("test error"))

		assert.Equal(t, 1, logger.getErrorCount())
		assert.True(t, span.errorRecorded)
	})

	t.Run("error without span", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)

		// 不应该 panic
		wrapper.logResult(context.Background(), nil, 100*time.Millisecond, errors.New("test error"))

		assert.Equal(t, 1, logger.getErrorCount())
	})
}

// ============================================================================
// Wrapper tryAcquireLock Edge Cases
// ============================================================================

func TestJobWrapper_TryAcquireLock_EdgeCases(t *testing.T) {
	t.Run("error during lock acquisition", func(t *testing.T) {
		locker := &errorLocker{err: errors.New("lock error")}
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), locker, logger, nil, opts)
		rh, lockErr := wrapper.tryAcquireLock(context.Background(), nil)

		assert.Nil(t, rh)        // 获取锁失败返回 nil
		assert.Error(t, lockErr) // 锁服务异常返回 error
		assert.Equal(t, 1, logger.getWarnCount())
	})

	t.Run("with lock timeout applies context deadline", func(t *testing.T) {
		// 使用一个会检查 ctx deadline 的 locker
		var receivedCtx context.Context
		locker := &ctxCapturingLocker{onTryLock: func(ctx context.Context) {
			receivedCtx = ctx
		}}
		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.lockTimeout = 2 * time.Second

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), locker, nil, nil, opts)
		rh, lockErr := wrapper.tryAcquireLock(context.Background(), nil)

		assert.NotNil(t, rh) // ctxCapturingLocker 返回成功
		assert.NoError(t, lockErr)
		require.NotNil(t, receivedCtx)
		deadline, ok := receivedCtx.Deadline()
		assert.True(t, ok, "lock context should have a deadline")
		assert.WithinDuration(t, time.Now().Add(2*time.Second), deadline, time.Second)
	})
}

// ctxCapturingLocker 捕获 TryLock 收到的 context
type ctxCapturingLocker struct {
	onTryLock func(ctx context.Context)
}

func (l *ctxCapturingLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	if l.onTryLock != nil {
		l.onTryLock(ctx)
	}
	return &noopLockHandle{key: key}, nil
}

var _ Locker = (*ctxCapturingLocker)(nil)

// errorLocker 返回锁获取错误的 mock locker
type errorLocker struct {
	err error
}

func (l *errorLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	return nil, l.err
}

var _ Locker = (*errorLocker)(nil)

// ============================================================================
// Wrapper executeJob Edge Cases
// ============================================================================

func TestJobWrapper_ExecuteJob_UnlockError(t *testing.T) {
	locker := &unlockErrorLocker{}
	logger := newMockLogger()
	opts := defaultJobOptions()
	opts.name = "test-job"

	wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), locker, logger, nil, opts)
	// 创建一个 mock renewHandle 来触发 unlock 流程
	rh := &renewHandle{
		cancel:     func() {},
		lockHandle: &unlockErrorLockHandle{key: "test-job"},
	}
	err := wrapper.executeJob(context.Background(), rh)

	assert.NoError(t, err)
	// Unlock 错误会被记录
	assert.Equal(t, 1, logger.getWarnCount())
}

// unlockErrorLocker 在 Unlock 时返回错误
type unlockErrorLocker struct{}

// unlockErrorLockHandle 在 Unlock 时返回错误的 LockHandle
type unlockErrorLockHandle struct {
	key string
}

func (l *unlockErrorLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	return &unlockErrorLockHandle{key: key}, nil
}

func (h *unlockErrorLockHandle) Unlock(ctx context.Context) error {
	return errors.New("unlock failed")
}

func (h *unlockErrorLockHandle) Renew(ctx context.Context, ttl time.Duration) error {
	return nil
}

func (h *unlockErrorLockHandle) Key() string {
	return h.key
}

var _ Locker = (*unlockErrorLocker)(nil)
var _ LockHandle = (*unlockErrorLockHandle)(nil)

// ============================================================================
// Wrapper startRenew Tests
// ============================================================================

func TestJobWrapper_StartRenew_EdgeCases(t *testing.T) {
	t.Run("with short TTL uses minimum interval", func(t *testing.T) {
		locker := NoopLocker()
		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.lockTTL = 1 * time.Second // TTL/3 < 1s，应该使用 1s

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), locker, nil, nil, opts)

		// 获取一个锁句柄
		lockHandle, _ := locker.TryLock(context.Background(), "test-job", time.Minute) //nolint:errcheck // NoopLocker 总是成功

		// startRenew 应该正常启动并返回 handle
		rh := wrapper.startRenew(context.Background(), nil, lockHandle)
		require.NotNil(t, rh)

		// 等待确保续期协程启动
		time.Sleep(100 * time.Millisecond)

		// stopRenew 应该正常停止
		wrapper.stopRenew(rh)
	})

	t.Run("renew error is logged", func(t *testing.T) {
		locker := &renewErrorLocker{}
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"
		opts.lockTTL = 3 * time.Second // interval = 1s

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), locker, logger, nil, opts)

		// 获取一个会在 Renew 时返回错误的锁句柄
		lockHandle, _ := locker.TryLock(context.Background(), "test-job", time.Minute) //nolint:errcheck // 测试专用 locker

		rh := wrapper.startRenew(context.Background(), nil, lockHandle)
		require.NotNil(t, rh)

		// 等待续期触发一次
		time.Sleep(1200 * time.Millisecond)
		wrapper.stopRenew(rh)

		// Renew 错误应该被记录（续期失败现在使用 Error 级别，因为会导致任务取消）
		assert.GreaterOrEqual(t, logger.getErrorCount(), 1)
	})
}

// renewErrorLocker 在 Renew 时返回错误
type renewErrorLocker struct{}

// renewErrorLockHandle 在 Renew 时返回错误的 LockHandle
type renewErrorLockHandle struct {
	key string
}

func (l *renewErrorLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	return &renewErrorLockHandle{key: key}, nil
}

func (h *renewErrorLockHandle) Unlock(ctx context.Context) error {
	return nil
}

func (h *renewErrorLockHandle) Renew(ctx context.Context, ttl time.Duration) error {
	return errors.New("renew failed")
}

func (h *renewErrorLockHandle) Key() string {
	return h.key
}

var _ Locker = (*renewErrorLocker)(nil)
var _ LockHandle = (*renewErrorLockHandle)(nil)

// ============================================================================
// WithImmediate Option Tests
// ============================================================================

func TestWithImmediate(t *testing.T) {
	t.Run("sets immediate flag to true", func(t *testing.T) {
		opts := defaultJobOptions()
		assert.False(t, opts.immediate) // 默认为 false

		WithImmediate()(opts)

		assert.True(t, opts.immediate)
	})

	t.Run("can be combined with other options", func(t *testing.T) {
		opts := defaultJobOptions()

		WithName("test-job")(opts)
		WithTimeout(30 * time.Second)(opts)
		WithImmediate()(opts)

		assert.Equal(t, "test-job", opts.name)
		assert.Equal(t, 30*time.Second, opts.timeout)
		assert.True(t, opts.immediate)
	})
}

func TestDefaultJobOptions_ImmediateIsFalse(t *testing.T) {
	opts := defaultJobOptions()
	assert.False(t, opts.immediate, "immediate should be false by default")
}
