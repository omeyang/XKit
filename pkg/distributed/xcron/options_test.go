package xcron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Mock Logger for testing
// ============================================================================

type mockLogger struct {
	mu        sync.Mutex
	debugMsgs []string
	warnMsgs  []string
	errorMsgs []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		debugMsgs: make([]string, 0),
		warnMsgs:  make([]string, 0),
		errorMsgs: make([]string, 0),
	}
}

func (l *mockLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugMsgs = append(l.debugMsgs, msg)
}

func (l *mockLogger) Info(ctx context.Context, msg string, args ...any) {}

func (l *mockLogger) Warn(ctx context.Context, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnMsgs = append(l.warnMsgs, msg)
}

func (l *mockLogger) Error(ctx context.Context, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errorMsgs = append(l.errorMsgs, msg)
}

func (l *mockLogger) getWarnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warnMsgs)
}

func (l *mockLogger) getErrorCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.errorMsgs)
}

func (l *mockLogger) getDebugCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.debugMsgs)
}

var _ Logger = (*mockLogger)(nil)

// ============================================================================
// Mock Observer for testing
// ============================================================================

type mockObserver struct {
	spanStarted atomic.Int32
}

func newMockObserver() *mockObserver {
	return &mockObserver{}
}

func (o *mockObserver) Start(ctx context.Context, name string, opts ...any) (context.Context, Span) {
	o.spanStarted.Add(1)
	return ctx, &mockSpan{}
}

type mockSpan struct {
	ended          bool
	errorRecorded  bool
	recordedErrors []error
}

func (s *mockSpan) End(opts ...any) { s.ended = true }
func (s *mockSpan) RecordError(err error, opts ...any) {
	s.errorRecorded = true
	s.recordedErrors = append(s.recordedErrors, err)
}
func (s *mockSpan) SetStatus(status any)       {}
func (s *mockSpan) SetAttributes(attrs ...any) {}

var _ Span = (*mockSpan)(nil)

// ============================================================================
// Mock RetryPolicy for testing
// ============================================================================

type mockRetryPolicy struct {
	maxRetries int
}

func newMockRetryPolicy(maxRetries int) *mockRetryPolicy {
	return &mockRetryPolicy{maxRetries: maxRetries}
}

func (p *mockRetryPolicy) ShouldRetry(attempt int, err error) bool {
	return attempt < p.maxRetries
}

var _ RetryPolicy = (*mockRetryPolicy)(nil)

// ============================================================================
// Mock BackoffPolicy for testing
// ============================================================================

type mockBackoffPolicy struct {
	backoffDuration time.Duration
}

func newMockBackoffPolicy(d time.Duration) *mockBackoffPolicy {
	return &mockBackoffPolicy{backoffDuration: d}
}

func (p *mockBackoffPolicy) NextDelay(attempt int) time.Duration {
	return p.backoffDuration
}

var _ BackoffPolicy = (*mockBackoffPolicy)(nil)

// ============================================================================
// Scheduler Options Tests
// ============================================================================

func TestWithLogger(t *testing.T) {
	t.Run("sets logger", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultSchedulerOptions()

		WithLogger(logger)(opts)

		assert.Equal(t, logger, opts.logger)
	})

	t.Run("allows nil logger", func(t *testing.T) {
		opts := defaultSchedulerOptions()
		opts.logger = newMockLogger()

		WithLogger(nil)(opts)

		assert.Nil(t, opts.logger)
	})
}

func TestWithParser(t *testing.T) {
	t.Run("sets custom parser", func(t *testing.T) {
		customParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		opts := defaultSchedulerOptions()

		WithParser(customParser)(opts)

		assert.Equal(t, customParser, opts.parser)
	})
}

func TestWithLocker_NilValue(t *testing.T) {
	opts := defaultSchedulerOptions()
	originalLocker := opts.locker

	// nil 值不应该覆盖原来的 locker
	WithLocker(nil)(opts)

	assert.Equal(t, originalLocker, opts.locker)
}

func TestWithLocation_NilValue(t *testing.T) {
	opts := defaultSchedulerOptions()
	originalLocation := opts.location

	// nil 值不应该覆盖原来的 location
	WithLocation(nil)(opts)

	assert.Equal(t, originalLocation, opts.location)
}

// ============================================================================
// Job Options Tests
// ============================================================================

func TestWithJobLocker(t *testing.T) {
	t.Run("sets job-level locker", func(t *testing.T) {
		locker := NoopLocker()
		opts := defaultJobOptions()

		WithJobLocker(locker)(opts)

		assert.Equal(t, locker, opts.locker)
	})

	t.Run("allows nil locker", func(t *testing.T) {
		opts := defaultJobOptions()
		opts.locker = NoopLocker()

		WithJobLocker(nil)(opts)

		assert.Nil(t, opts.locker)
	})
}

func TestWithLockTTL(t *testing.T) {
	t.Run("sets positive TTL", func(t *testing.T) {
		opts := defaultJobOptions()

		WithLockTTL(10 * time.Minute)(opts)

		assert.Equal(t, 10*time.Minute, opts.lockTTL)
	})

	t.Run("ignores zero TTL", func(t *testing.T) {
		opts := defaultJobOptions()
		originalTTL := opts.lockTTL

		WithLockTTL(0)(opts)

		assert.Equal(t, originalTTL, opts.lockTTL)
	})

	t.Run("ignores negative TTL", func(t *testing.T) {
		opts := defaultJobOptions()
		originalTTL := opts.lockTTL

		WithLockTTL(-1 * time.Minute)(opts)

		assert.Equal(t, originalTTL, opts.lockTTL)
	})
}

func TestWithRetry(t *testing.T) {
	t.Run("sets retry policy", func(t *testing.T) {
		policy := newMockRetryPolicy(3)
		opts := defaultJobOptions()

		WithRetry(policy)(opts)

		assert.Equal(t, policy, opts.retry)
	})

	t.Run("allows nil policy", func(t *testing.T) {
		opts := defaultJobOptions()

		WithRetry(nil)(opts)

		assert.Nil(t, opts.retry)
	})
}

func TestWithBackoff(t *testing.T) {
	t.Run("sets backoff policy", func(t *testing.T) {
		policy := newMockBackoffPolicy(100 * time.Millisecond)
		opts := defaultJobOptions()

		WithBackoff(policy)(opts)

		assert.Equal(t, policy, opts.backoff)
	})

	t.Run("allows nil policy", func(t *testing.T) {
		opts := defaultJobOptions()

		WithBackoff(nil)(opts)

		assert.Nil(t, opts.backoff)
	})
}

func TestWithTracer(t *testing.T) {
	t.Run("sets tracer", func(t *testing.T) {
		tracer := newMockObserver()
		opts := defaultJobOptions()

		WithTracer(tracer)(opts)

		assert.Equal(t, tracer, opts.tracer)
	})

	t.Run("allows nil tracer", func(t *testing.T) {
		opts := defaultJobOptions()

		WithTracer(nil)(opts)

		assert.Nil(t, opts.tracer)
	})
}

func TestWithTimeout_EdgeCases(t *testing.T) {
	t.Run("ignores zero timeout", func(t *testing.T) {
		opts := defaultJobOptions()
		originalTimeout := opts.timeout

		WithTimeout(0)(opts)

		assert.Equal(t, originalTimeout, opts.timeout)
	})

	t.Run("ignores negative timeout", func(t *testing.T) {
		opts := defaultJobOptions()
		originalTimeout := opts.timeout

		WithTimeout(-1 * time.Second)(opts)

		assert.Equal(t, originalTimeout, opts.timeout)
	})
}

// ============================================================================
// Default Options Tests
// ============================================================================

func TestDefaultSchedulerOptions(t *testing.T) {
	opts := defaultSchedulerOptions()

	assert.NotNil(t, opts.locker)
	assert.Nil(t, opts.logger)
	assert.Equal(t, time.Local, opts.location)
	assert.NotNil(t, opts.parser)
}

func TestDefaultJobOptions(t *testing.T) {
	opts := defaultJobOptions()

	assert.Empty(t, opts.name)
	assert.Nil(t, opts.locker)
	assert.Equal(t, 5*time.Minute, opts.lockTTL)
	assert.Equal(t, time.Duration(0), opts.timeout)
	assert.Nil(t, opts.retry)
	assert.Nil(t, opts.backoff)
	assert.Nil(t, opts.tracer)
}

// ============================================================================
// Integration: Scheduler with Options
// ============================================================================

func TestScheduler_WithLogger(t *testing.T) {
	logger := newMockLogger()
	s := New(WithLogger(logger))
	defer s.Stop()

	require.NotNil(t, s)
}

func TestScheduler_WithCustomParser(t *testing.T) {
	// 使用支持秒级的解析器
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	s := New(WithParser(parser))
	defer s.Stop()

	// 验证可以解析秒级表达式
	_, err := s.AddFunc("*/5 * * * * *", func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

// ============================================================================
// Integration: Job with All Options
// ============================================================================

func TestJob_WithAllOptions(t *testing.T) {
	logger := newMockLogger()
	s := New(WithLogger(logger))
	defer s.Stop()

	tracer := newMockObserver()
	locker := NoopLocker()
	retry := newMockRetryPolicy(3)
	backoff := newMockBackoffPolicy(10 * time.Millisecond)

	var executed atomic.Bool
	id, err := s.AddFunc("@every 1s", func(ctx context.Context) error {
		executed.Store(true)
		return nil
	},
		WithName("full-options-job"),
		WithJobLocker(locker),
		WithLockTTL(10*time.Minute),
		WithTimeout(30*time.Second),
		WithRetry(retry),
		WithBackoff(backoff),
		WithTracer(tracer),
	)

	require.NoError(t, err)
	assert.NotZero(t, id)

	// 启动并运行一小段时间
	s.Start()
	time.Sleep(1200 * time.Millisecond)
	s.Stop()

	// 验证任务被执行
	assert.True(t, executed.Load())
	// 验证 tracer 被调用
	assert.GreaterOrEqual(t, tracer.spanStarted.Load(), int32(1))
}

// ============================================================================
// Wrapper Logging Tests
// ============================================================================

func TestJobWrapper_LogWarn(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)

		wrapper.logWarn(context.Background(), "test warning", "key", "value")

		assert.Equal(t, 1, logger.getWarnCount())
	})

	t.Run("without logger uses default log", func(t *testing.T) {
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), nil, nil, opts)

		// 不应该 panic
		wrapper.logWarn(context.Background(), "test warning", "key", "value")
	})
}

func TestJobWrapper_LogError(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)

		wrapper.logError(context.Background(), "test error", "key", "value")

		assert.Equal(t, 1, logger.getErrorCount())
	})

	t.Run("without logger uses default log", func(t *testing.T) {
		opts := defaultJobOptions()
		opts.name = "test-job"
		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), nil, nil, opts)

		// 不应该 panic
		wrapper.logError(context.Background(), "test error", "key", "value")
	})
}

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
}

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

		wrapper.logResult(context.Background(), span, nil)

		assert.Equal(t, 1, logger.getDebugCount())
		assert.False(t, span.errorRecorded)
	})

	t.Run("error with span", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)
		span := &mockSpan{}

		wrapper.logResult(context.Background(), span, errors.New("test error"))

		assert.Equal(t, 1, logger.getErrorCount())
		assert.True(t, span.errorRecorded)
	})

	t.Run("error without span", func(t *testing.T) {
		logger := newMockLogger()
		opts := defaultJobOptions()
		opts.name = "test-job"

		wrapper := newJobWrapper(JobFunc(func(ctx context.Context) error { return nil }), NoopLocker(), logger, nil, opts)

		// 不应该 panic
		wrapper.logResult(context.Background(), nil, errors.New("test error"))

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
		rh := wrapper.tryAcquireLock(context.Background(), nil)

		assert.Nil(t, rh) // 获取锁失败返回 nil
		assert.Equal(t, 1, logger.getWarnCount())
	})
}

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
