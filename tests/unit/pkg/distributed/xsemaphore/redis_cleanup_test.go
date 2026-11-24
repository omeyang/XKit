package xsemaphore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

// =============================================================================
// backgroundCleanupLoop 测试
// =============================================================================

func TestBackgroundCleanupLoop_StopsOnClose(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)

	// Close should stop the background cleanup
	err := sem.Close(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Second close should be idempotent
	err = sem.Close(context.Background())
	if err != nil {
		t.Errorf("unexpected error on second close: %v", err)
	}
}

// =============================================================================
// applyDefaultTimeout nil context 回归测试（FG-S1 修复验证）
// =============================================================================

func TestApplyDefaultTimeout_NilContext(t *testing.T) {
	// 修复前：当 defaultTimeout > 0 且 ctx == nil 时，ctx.Deadline() 会 panic
	// 修复后：nil ctx 直接透传，由后续 validateCommonParams 返回 ErrNilContext
	t.Run("nil ctx with positive timeout does not panic", func(t *testing.T) {
		ctx, cancel := applyDefaultTimeout(nil, 5*time.Second)
		defer cancel()
		assert.Nil(t, ctx)
	})

	t.Run("nil ctx with zero timeout returns nil", func(t *testing.T) {
		ctx, cancel := applyDefaultTimeout(nil, 0)
		defer cancel()
		assert.Nil(t, ctx)
	})
}

// TestNilContextWithDefaultTimeout_Redis 验证 Redis 信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Redis(t *testing.T) {
	_, client := setupRedis(t)

	sem, err := New(client, WithDefaultTimeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { closeSemaphore(t, sem) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// TestNilContextWithDefaultTimeout_Local 验证本地信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Local(t *testing.T) {
	opts := defaultOptions()
	opts.defaultTimeout = 5 * time.Second
	sem := newLocalSemaphore(opts)
	t.Cleanup(func() { _ = sem.Close(context.Background()) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// TestNilContextWithDefaultTimeout_Fallback 验证降级信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Fallback(t *testing.T) {
	_, client := setupRedis(t)

	sem, err := New(client,
		WithDefaultTimeout(5*time.Second),
		WithFallback(FallbackLocal),
		WithPodCount(1),
	)
	require.NoError(t, err)
	t.Cleanup(func() { closeSemaphore(t, sem) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// =============================================================================
// ErrNilContext 测试（覆盖新增的 nil context 校验路径）
// =============================================================================

func TestNilContext_ValidateCommonParams(t *testing.T) {
	err := validateCommonParams(nil, "resource", false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_PrepareAcquireCommon(t *testing.T) {
	_, _, err := prepareAcquireCommon(nil, "resource", nil, false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_PrepareQueryCommon(t *testing.T) {
	_, _, err := prepareQueryCommon(nil, "resource", nil, false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_ReleaseCommon(t *testing.T) {
	p := &permitBase{}
	initPermitBase(p, "test-id", "resource", "tenant", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)
	err := p.releaseCommon(nil, nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_ExtendCommon(t *testing.T) {
	p := &permitBase{}
	initPermitBase(p, "test-id", "resource", "tenant", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)
	err := p.extendCommon(nil, nil, SemaphoreTypeLocal, func(ctx context.Context, _ time.Time) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_NoopPermitRelease(t *testing.T) {
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, defaultOptions())
	require.NoError(t, err)
	assert.ErrorIs(t, p.Release(nil), ErrNilContext)
}

func TestNilContext_NoopPermitExtend(t *testing.T) {
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, defaultOptions())
	require.NoError(t, err)
	assert.ErrorIs(t, p.Extend(nil), ErrNilContext)
}

// =============================================================================
// noopPermit logExtendFailed 测试
// =============================================================================

func TestNoopPermit_LogExtendFailed(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := defaultOptions()
		opts.logger = logger
		p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
		require.NoError(t, err)
		p.logExtendFailed(context.Background(), p.ID(), "resource", errors.New("test error"))
		assert.True(t, logger.warnCalled, "expected Warn to be called")
	})

	t.Run("without logger", func(t *testing.T) {
		opts := defaultOptions()
		opts.logger = nil
		p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
		require.NoError(t, err)
		// Should not panic
		p.logExtendFailed(context.Background(), p.ID(), "resource", errors.New("test error"))
	})
}

// =============================================================================
// newNoopPermit ID 生成失败测试
// =============================================================================

func TestNewNoopPermit_IDGenerationFailure(t *testing.T) {
	opts := defaultOptions()
	opts.idGenerator = func(_ context.Context) (string, error) {
		return "", errors.New("id gen failed")
	}
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, ErrIDGenerationFailed)
}

// =============================================================================
// Sentinel error wrapping 测试
// =============================================================================

func TestOptions_Validate_SentinelErrors(t *testing.T) {
	t.Run("invalid pod count wraps ErrInvalidPodCount", func(t *testing.T) {
		opts := defaultOptions()
		opts.podCount = -1
		err := opts.validate()
		assert.ErrorIs(t, err, ErrInvalidPodCount)
	})

	t.Run("invalid fallback strategy wraps ErrInvalidFallbackStrategy", func(t *testing.T) {
		opts := defaultOptions()
		opts.fallback = FallbackStrategy("invalid")
		err := opts.validate()
		assert.ErrorIs(t, err, ErrInvalidFallbackStrategy)
	})
}

// =============================================================================
// convertScriptResult 测试（FG-M4 覆盖率修复）
// =============================================================================

func TestConvertScriptResult(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  []int64
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "int64 elements",
			input:    []any{int64(1), int64(2), int64(3)},
			expected: []int64{1, 2, 3},
		},
		{
			name:     "int elements",
			input:    []any{int(10), int(20)},
			expected: []int64{10, 20},
		},
		{
			name:     "float64 integer values",
			input:    []any{float64(42), float64(100)},
			expected: []int64{42, 100},
		},
		{
			name:     "mixed numeric types",
			input:    []any{int64(1), int(2), float64(3)},
			expected: []int64{1, 2, 3},
		},
		{
			name:     "empty array",
			input:    []any{},
			expected: []int64{},
		},
		{
			name:      "non-array input",
			input:     "not an array",
			wantErr:   true,
			errSubstr: "expected array",
		},
		{
			name:      "nil input",
			input:     nil,
			wantErr:   true,
			errSubstr: "expected array",
		},
		{
			name:      "float64 non-integer value",
			input:     []any{float64(3.14)},
			wantErr:   true,
			errSubstr: "non-integer float64",
		},
		{
			name:      "float64 non-integer negative",
			input:     []any{float64(-2.5)},
			wantErr:   true,
			errSubstr: "non-integer float64",
		},
		{
			name:      "unknown type string",
			input:     []any{"not a number"},
			wantErr:   true,
			errSubstr: "expected number",
		},
		{
			name:      "unknown type bool",
			input:     []any{true},
			wantErr:   true,
			errSubstr: "expected number",
		},
		{
			name:      "mixed with unknown type in middle",
			input:     []any{int64(1), "bad", int64(3)},
			wantErr:   true,
			errSubstr: "element 1 is string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertScriptResult(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUnexpectedScriptResult)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// releasePermit / extendPermit default 分支测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRedisSemaphore_ReleasePermit_UnknownStatus(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			// 返回未知状态码 99
			release: redis.NewScript(`return {99, 0}`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)

	err := sem.releasePermit(context.Background(), p)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownScriptStatus)
	assert.Contains(t, err.Error(), "release returned status 99")
}

func TestRedisSemaphore_ReleasePermit_UnknownStatus_WithTenant(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			release: redis.NewScript(`return {99, 0}`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "tenant-1", time.Now().Add(5*time.Minute), 5*time.Minute, true, nil)

	err := sem.releasePermit(context.Background(), p)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownScriptStatus)
}

func TestRedisSemaphore_ExtendPermit_UnknownStatus(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			// 返回未知状态码 99
			extend: redis.NewScript(`return {99}`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)

	err := sem.extendPermit(context.Background(), p, time.Now().Add(10*time.Minute))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownScriptStatus)
	assert.Contains(t, err.Error(), "extend returned status 99")
}

func TestRedisSemaphore_ExtendPermit_UnknownStatus_WithTenant(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			extend: redis.NewScript(`return {99}`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "tenant-1", time.Now().Add(5*time.Minute), 5*time.Minute, true, nil)

	err := sem.extendPermit(context.Background(), p, time.Now().Add(10*time.Minute))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownScriptStatus)
}

// =============================================================================
// recordAcquireMetrics 带 metrics 路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRedisSemaphore_Acquire_WithMetrics(t *testing.T) {
	_, client := setupRedis(t)

	mp := noop.NewMeterProvider()
	sem, err := New(client, WithMeterProvider(mp))
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	// 成功路径：recordAcquireMetrics 在 Acquire 中被调用
	permit, err := sem.Acquire(ctx, "test-resource",
		WithCapacity(10),
		WithMaxRetries(3),
		WithRetryDelay(10*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)
	releasePermit(t, ctx, permit)

	// 失败路径：容量满时 Acquire 重试耗尽
	// 先占满容量
	permits := make([]Permit, 2)
	for i := range permits {
		p, err := sem.TryAcquire(ctx, "full-resource", WithCapacity(2))
		require.NoError(t, err)
		require.NotNil(t, p)
		permits[i] = p
	}

	// Acquire 应失败
	_, err = sem.Acquire(ctx, "full-resource",
		WithCapacity(2),
		WithMaxRetries(2),
		WithRetryDelay(10*time.Millisecond),
	)
	assert.ErrorIs(t, err, ErrAcquireFailed)

	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
}

// =============================================================================
// localSemaphore Acquire + Extend 带 metrics 路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestLocalSemaphore_Acquire_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	// 成功路径
	permit, err := sem.Acquire(ctx, "test-resource",
		WithCapacity(10),
		WithMaxRetries(3),
		WithRetryDelay(10*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// Extend 路径（测试 recordExtendMetrics with non-nil metrics）
	err = permit.Extend(ctx)
	require.NoError(t, err)

	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestLocalSemaphore_Acquire_Exhausted_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	// 占满容量
	p, err := sem.TryAcquire(ctx, "full-resource", WithCapacity(1))
	require.NoError(t, err)
	require.NotNil(t, p)
	defer releasePermit(t, ctx, p)

	// Acquire 失败路径
	_, err = sem.Acquire(ctx, "full-resource",
		WithCapacity(1),
		WithMaxRetries(2),
		WithRetryDelay(10*time.Millisecond),
	)
	assert.ErrorIs(t, err, ErrAcquireFailed)
}

// =============================================================================
// recordFallbackObservability 带 metrics 路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRecordFallbackObservability_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	logger := &testLogger{}
	opts := &options{
		metrics:  metrics,
		logger:   logger,
		fallback: FallbackLocal,
	}

	f := &fallbackSemaphore{
		strategy: FallbackLocal,
		opts:     opts,
	}

	f.recordFallbackObservability(context.Background(), "resource", ErrRedisUnavailable)

	assert.True(t, logger.warnCalled, "expected logger.Warn to be called")
}

// =============================================================================
// Health 关闭状态路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRedisSemaphore_Health_Closed(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
	}
	sem.closed.Store(true)

	err := sem.Health(context.Background())
	assert.ErrorIs(t, err, ErrSemaphoreClosed)
}

func TestLocalSemaphore_Health_Closed(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	_ = sem.Close(context.Background())

	err := sem.Health(context.Background())
	assert.ErrorIs(t, err, ErrSemaphoreClosed)
}

func TestRedisSemaphore_Health_NilContext(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
	}

	err := sem.Health(nil) //nolint:staticcheck // 测试 nil context 路径
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestLocalSemaphore_Health_NilContext(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	err := sem.Health(nil) //nolint:staticcheck // 测试 nil context 路径
	assert.ErrorIs(t, err, ErrNilContext)
}

// =============================================================================
// extendPermit NotHeld 路径带 metrics 测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRedisSemaphore_ExtendPermit_NotHeld_WithMetrics(t *testing.T) {
	_, client := setupRedis(t)

	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics

	sem := &redisSemaphore{
		client: client,
		opts:   opts,
		scripts: &scripts{
			// scriptStatusNotHeld = 3
			extend: redis.NewScript(`return {3}`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)

	err = sem.extendPermit(context.Background(), p, time.Now().Add(10*time.Minute))
	assert.ErrorIs(t, err, ErrPermitNotHeld)
}

func TestRedisSemaphore_ExtendPermit_ScriptError(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			extend: redis.NewScript(`return redis.error_reply("test error")`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)

	err := sem.extendPermit(context.Background(), p, time.Now().Add(10*time.Minute))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extend script failed")
}

func TestRedisSemaphore_ReleasePermit_ScriptError(t *testing.T) {
	_, client := setupRedis(t)

	sem := &redisSemaphore{
		client: client,
		opts:   defaultOptions(),
		scripts: &scripts{
			release: redis.NewScript(`return redis.error_reply("test error")`),
		},
	}

	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, "test-permit", "test-resource", "", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)

	err := sem.releasePermit(context.Background(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release script failed")
}

// =============================================================================
// noopPermit Release/Extend 带 metrics 路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestNoopPermit_Release_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics

	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
	require.NoError(t, err)

	err = p.Release(context.Background())
	assert.NoError(t, err)
}

func TestNoopPermit_Extend_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics

	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
	require.NoError(t, err)

	err = p.Extend(context.Background())
	assert.NoError(t, err)
}

// =============================================================================
// handleQueryError 带 metrics 路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestRedisSemaphore_HandleQueryError_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics

	sem := &redisSemaphore{opts: opts}

	// 模拟 nil span（从 Background context 获取）
	ctx := context.Background()
	_, span := startSpan(ctx, nil, "test")
	defer span.End()

	resultErr := sem.handleQueryError(ctx, span, "resource", time.Now(), errors.New("test error"))
	require.Error(t, resultErr)
	assert.Contains(t, resultErr.Error(), "query script failed")
}

// =============================================================================
// local.Acquire context 取消路径测试（FG-M3/M4 覆盖率修复）
// =============================================================================

func TestLocalSemaphore_Acquire_ContextCanceled_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	// 占满容量
	p, err := sem.TryAcquire(context.Background(), "resource", WithCapacity(1))
	require.NoError(t, err)
	require.NotNil(t, p)
	defer releasePermit(t, context.Background(), p)

	// 已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = sem.Acquire(ctx, "resource",
		WithCapacity(1),
		WithMaxRetries(5),
		WithRetryDelay(10*time.Millisecond),
	)
	assert.Error(t, err)
}
