package xcron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_Basic(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())

	require.NotNil(t, result)
	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.Equal(t, 0, result.RegisteredJobs)
	assert.Equal(t, int64(0), result.TotalExecutions)
	assert.Contains(t, result.Message, "no jobs registered")
	assert.NotZero(t, result.CheckTime)
}

func TestHealthCheck_WithJobs(t *testing.T) {
	scheduler := New(WithSeconds())
	defer scheduler.Stop()

	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		return nil
	}, WithName("health-test-job"))
	require.NoError(t, err)

	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())

	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.Equal(t, 1, result.RegisteredJobs)
	assert.True(t, result.HasJobs)
}

func TestHealthCheck_AfterExecution(t *testing.T) {
	scheduler := New(WithSeconds())
	defer scheduler.Stop()

	executed := make(chan struct{}, 1)
	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		select {
		case executed <- struct{}{}:
		default:
		}
		return nil
	}, WithName("health-exec-job"))
	require.NoError(t, err)

	scheduler.Start()

	// 等待执行
	select {
	case <-executed:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for execution")
	}

	// 等待统计更新
	time.Sleep(100 * time.Millisecond)

	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())

	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.GreaterOrEqual(t, result.TotalExecutions, int64(1))
	assert.GreaterOrEqual(t, result.SuccessCount, int64(1))
	assert.Equal(t, int64(0), result.FailureCount)
	assert.Equal(t, float64(1), result.SuccessRate)
}

func TestHealthCheck_WithFailures(t *testing.T) {
	scheduler := New(WithSeconds())
	defer scheduler.Stop()

	// 直接操作 stats 模拟失败
	stats := scheduler.Stats()
	for i := 0; i < 10; i++ {
		stats.recordExecution("test-job", time.Millisecond, nil)
	}
	for i := 0; i < 10; i++ {
		stats.recordExecution("test-job", time.Millisecond, errors.New("error"))
	}

	// 50% 失败率，刚好等于阈值
	checker := NewHealthChecker(scheduler, WithDegradedThreshold(0.5))
	result := checker.Check(context.Background())

	// 50% 失败率不应触发 degraded（需要 > threshold）
	assert.Equal(t, HealthStatusHealthy, result.Status)
}

func TestHealthCheck_Degraded(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	// 直接操作 stats 模拟高失败率
	stats := scheduler.Stats()
	for i := 0; i < 3; i++ {
		stats.recordExecution("test-job", time.Millisecond, nil)
	}
	for i := 0; i < 10; i++ {
		stats.recordExecution("test-job", time.Millisecond, errors.New("error"))
	}

	// 约 77% 失败率，应触发 degraded
	checker := NewHealthChecker(scheduler, WithDegradedThreshold(0.5), WithMinExecutions(10))
	result := checker.Check(context.Background())

	assert.Equal(t, HealthStatusDegraded, result.Status)
	assert.Contains(t, result.Message, "high failure rate")
}

func TestHealthCheck_MinExecutions(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	// 100% 失败率，但执行次数不够
	stats := scheduler.Stats()
	for i := 0; i < 5; i++ {
		stats.recordExecution("test-job", time.Millisecond, errors.New("error"))
	}

	checker := NewHealthChecker(scheduler, WithMinExecutions(10))
	result := checker.Check(context.Background())

	// 执行次数低于阈值，不应判断为 degraded
	assert.Equal(t, HealthStatusHealthy, result.Status)
}

func TestHealthCheck_LastError(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	expectedErr := errors.New("last error message")
	stats := scheduler.Stats()
	stats.recordExecution("test-job", time.Millisecond, expectedErr)

	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())

	assert.Equal(t, "last error message", result.LastError)
}

func TestHealthCheck_Details(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	stats := scheduler.Stats()
	stats.recordExecution("test-job", 100*time.Millisecond, nil)
	stats.recordExecution("test-job", 200*time.Millisecond, nil)
	stats.recordSkip("test-job")

	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())

	require.NotNil(t, result.Details)
	assert.Equal(t, int64(1), result.Details["skip_count"])
	assert.NotEmpty(t, result.Details["min_duration"])
	assert.NotEmpty(t, result.Details["max_duration"])
	assert.NotEmpty(t, result.Details["avg_duration"])
}

// mockLockerWithHealth 实现了 LockerHealthChecker 接口的 mock
type mockLockerWithHealth struct {
	*noopLocker
	healthErr error
}

func (m *mockLockerWithHealth) Health(ctx context.Context) error {
	return m.healthErr
}

var _ LockerHealthChecker = (*mockLockerWithHealth)(nil)

func TestHealthCheck_LockerHealth(t *testing.T) {
	t.Run("locker healthy", func(t *testing.T) {
		locker := &mockLockerWithHealth{noopLocker: &noopLocker{}, healthErr: nil}
		scheduler := New(WithLocker(locker))
		defer scheduler.Stop()

		checker := NewHealthChecker(scheduler, WithCheckLocker())
		result := checker.Check(context.Background())

		assert.Equal(t, HealthStatusHealthy, result.Status)
	})

	t.Run("locker unhealthy", func(t *testing.T) {
		locker := &mockLockerWithHealth{noopLocker: &noopLocker{}, healthErr: errors.New("connection failed")}
		scheduler := New(WithLocker(locker))
		defer scheduler.Stop()

		checker := NewHealthChecker(scheduler, WithCheckLocker())
		result := checker.Check(context.Background())

		assert.Equal(t, HealthStatusUnhealthy, result.Status)
		assert.Contains(t, result.Message, "locker unhealthy")
		assert.Contains(t, result.Message, "connection failed")
	})
}

func TestHealthCheck_LockerWithoutHealthInterface(t *testing.T) {
	// NoopLocker 没有实现 LockerHealthChecker
	scheduler := New(WithLocker(NoopLocker()))
	defer scheduler.Stop()

	checker := NewHealthChecker(scheduler, WithCheckLocker())
	result := checker.Check(context.Background())

	// 应该仍然是 healthy，因为 locker 没有实现健康检查接口
	assert.Equal(t, HealthStatusHealthy, result.Status)
}

func TestHealthCheck_Options(t *testing.T) {
	t.Run("WithDegradedThreshold", func(t *testing.T) {
		opts := defaultHealthCheckOptions()
		WithDegradedThreshold(0.3)(opts)
		assert.Equal(t, 0.3, opts.degradedThreshold)

		// 无效值应被忽略
		WithDegradedThreshold(-0.1)(opts)
		assert.Equal(t, 0.3, opts.degradedThreshold)

		WithDegradedThreshold(1.5)(opts)
		assert.Equal(t, 0.3, opts.degradedThreshold)
	})

	t.Run("WithMinExecutions", func(t *testing.T) {
		opts := defaultHealthCheckOptions()
		WithMinExecutions(100)(opts)
		assert.Equal(t, int64(100), opts.minExecutions)

		// 无效值应被忽略
		WithMinExecutions(-1)(opts)
		assert.Equal(t, int64(100), opts.minExecutions)
	})

	t.Run("WithCheckLocker", func(t *testing.T) {
		opts := defaultHealthCheckOptions()
		assert.False(t, opts.checkLocker)
		WithCheckLocker()(opts)
		assert.True(t, opts.checkLocker)
	})
}

func TestHealthStatus_String(t *testing.T) {
	assert.Equal(t, "healthy", string(HealthStatusHealthy))
	assert.Equal(t, "degraded", string(HealthStatusDegraded))
	assert.Equal(t, "unhealthy", string(HealthStatusUnhealthy))
}

func TestHealthCheck_Concurrent(t *testing.T) {
	scheduler := New(WithSeconds())
	defer scheduler.Stop()

	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		return nil
	}, WithName("concurrent-health-job"))
	require.NoError(t, err)

	scheduler.Start()

	checker := NewHealthChecker(scheduler)

	// 并发调用健康检查
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					result := checker.Check(context.Background())
					assert.NotNil(t, result)
				}
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)
	close(done)
}

func TestHealthCheck_CheckTime(t *testing.T) {
	scheduler := New()
	defer scheduler.Stop()

	before := time.Now()
	checker := NewHealthChecker(scheduler)
	result := checker.Check(context.Background())
	after := time.Now()

	assert.True(t, result.CheckTime.After(before) || result.CheckTime.Equal(before))
	assert.True(t, result.CheckTime.Before(after) || result.CheckTime.Equal(after))
}

func TestNewHealthChecker_InvalidScheduler(t *testing.T) {
	// 测试 fallbackHealthChecker 的行为
	fallback := &fallbackHealthChecker{message: "test error"}
	result := fallback.Check(context.Background())

	assert.Equal(t, HealthStatusUnhealthy, result.Status)
	assert.False(t, result.HasJobs)
	assert.Equal(t, "test error", result.Message)
}
