package xcron

import (
	"context"
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

func TestWithLockTimeout(t *testing.T) {
	t.Run("sets positive lock timeout", func(t *testing.T) {
		opts := defaultJobOptions()

		WithLockTimeout(5 * time.Second)(opts)

		assert.Equal(t, 5*time.Second, opts.lockTimeout)
	})

	t.Run("ignores zero lock timeout", func(t *testing.T) {
		opts := defaultJobOptions()

		WithLockTimeout(0)(opts)

		assert.Equal(t, 5*time.Second, opts.lockTimeout) // 保持默认值
	})

	t.Run("ignores negative lock timeout", func(t *testing.T) {
		opts := defaultJobOptions()

		WithLockTimeout(-1 * time.Second)(opts)

		assert.Equal(t, 5*time.Second, opts.lockTimeout) // 保持默认值
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
	assert.Equal(t, 5*time.Second, opts.lockTimeout) // 默认锁获取超时
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
