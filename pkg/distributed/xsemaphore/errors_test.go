package xsemaphore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestSentinelErrors(t *testing.T) {
	// 验证所有哨兵错误都已定义
	assert.NotNil(t, ErrNilClient)
	assert.NotNil(t, ErrSemaphoreClosed)
	assert.NotNil(t, ErrCapacityFull)
	assert.NotNil(t, ErrTenantQuotaExceeded)
	assert.NotNil(t, ErrPermitNotHeld)
	assert.NotNil(t, ErrAcquireFailed)
	assert.NotNil(t, ErrInvalidCapacity)
	assert.NotNil(t, ErrIDGenerationFailed)

	// 验证错误信息
	assert.Contains(t, ErrNilClient.Error(), "nil")
	assert.Contains(t, ErrSemaphoreClosed.Error(), "closed")
	assert.Contains(t, ErrCapacityFull.Error(), "capacity")
	assert.Contains(t, ErrTenantQuotaExceeded.Error(), "quota")
	assert.Contains(t, ErrPermitNotHeld.Error(), "not held")
	assert.Contains(t, ErrIDGenerationFailed.Error(), "generate permit ID")
}

func TestErrIDGenerationFailed_Wrapping(t *testing.T) {
	// 验证 ErrIDGenerationFailed 可以被正确包装和解包
	wrappedErr := fmt.Errorf("%w: clock backward timeout", ErrIDGenerationFailed)
	assert.True(t, errors.Is(wrappedErr, ErrIDGenerationFailed))
	assert.Contains(t, wrappedErr.Error(), "generate permit ID")
	assert.Contains(t, wrappedErr.Error(), "clock backward")
}

func TestAcquireFailReason_String(t *testing.T) {
	tests := []struct {
		reason   AcquireFailReason
		expected string
	}{
		{ReasonUnknown, "unknown"},
		{ReasonCapacityFull, "capacity_full"},
		{ReasonTenantQuotaExceeded, "tenant_quota_exceeded"},
		{AcquireFailReason(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.reason.String())
		})
	}
}

func TestAcquireFailReason_Error(t *testing.T) {
	tests := []struct {
		reason   AcquireFailReason
		expected error
	}{
		{ReasonUnknown, nil},
		{ReasonCapacityFull, ErrCapacityFull},
		{ReasonTenantQuotaExceeded, ErrTenantQuotaExceeded},
		{AcquireFailReason(99), nil},
	}

	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.reason.Error())
		})
	}
}

func TestIsCapacityFull(t *testing.T) {
	assert.True(t, IsCapacityFull(ErrCapacityFull))
	assert.True(t, IsCapacityFull(fmt.Errorf("wrapped: %w", ErrCapacityFull)))
	assert.False(t, IsCapacityFull(ErrPermitNotHeld))
	assert.False(t, IsCapacityFull(nil))
	assert.False(t, IsCapacityFull(errors.New("other error")))
}

func TestIsTenantQuotaExceeded(t *testing.T) {
	assert.True(t, IsTenantQuotaExceeded(ErrTenantQuotaExceeded))
	assert.True(t, IsTenantQuotaExceeded(fmt.Errorf("wrapped: %w", ErrTenantQuotaExceeded)))
	assert.False(t, IsTenantQuotaExceeded(ErrCapacityFull))
	assert.False(t, IsTenantQuotaExceeded(nil))
	assert.False(t, IsTenantQuotaExceeded(errors.New("other error")))
}

func TestIsPermitNotHeld(t *testing.T) {
	assert.True(t, IsPermitNotHeld(ErrPermitNotHeld))
	assert.True(t, IsPermitNotHeld(fmt.Errorf("wrapped: %w", ErrPermitNotHeld)))
	assert.False(t, IsPermitNotHeld(ErrCapacityFull))
	assert.False(t, IsPermitNotHeld(nil))
	assert.False(t, IsPermitNotHeld(errors.New("other error")))
}

func TestIsRedisError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsRedisError(nil))
	})

	t.Run("regular error", func(t *testing.T) {
		assert.False(t, IsRedisError(errors.New("some error")))
	})

	t.Run("network error", func(t *testing.T) {
		// 模拟网络错误
		netErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}
		assert.True(t, IsRedisError(netErr))
	})

	t.Run("wrapped network error", func(t *testing.T) {
		netErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}
		wrapped := fmt.Errorf("redis error: %w", netErr)
		assert.True(t, IsRedisError(wrapped))
	})

	t.Run("timeout error", func(t *testing.T) {
		// 模拟超时错误
		timeoutErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: &timeoutError{},
		}
		assert.True(t, IsRedisError(timeoutErr))
	})
}

// timeoutError 用于测试超时错误
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestIsRetryable(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsRetryable(nil))
	})

	// 容量/配额错误不可重试（需要等待释放）
	t.Run("capacity full is not retryable", func(t *testing.T) {
		assert.False(t, IsRetryable(ErrCapacityFull))
	})

	t.Run("tenant quota exceeded is not retryable", func(t *testing.T) {
		assert.False(t, IsRetryable(ErrTenantQuotaExceeded))
	})

	t.Run("wrapped capacity full is not retryable", func(t *testing.T) {
		wrapped := fmt.Errorf("failed: %w", ErrCapacityFull)
		assert.False(t, IsRetryable(wrapped))
	})

	t.Run("permit not held is not retryable", func(t *testing.T) {
		assert.False(t, IsRetryable(ErrPermitNotHeld))
	})

	t.Run("semaphore closed is not retryable", func(t *testing.T) {
		assert.False(t, IsRetryable(ErrSemaphoreClosed))
	})

	t.Run("regular error is not retryable", func(t *testing.T) {
		assert.False(t, IsRetryable(errors.New("some error")))
	})

	// Redis 网络错误可重试
	t.Run("network error is retryable", func(t *testing.T) {
		netErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}
		assert.True(t, IsRetryable(netErr))
	})
}

func TestErrorWrapping(t *testing.T) {
	t.Run("errors.Is works correctly", func(t *testing.T) {
		wrapped := fmt.Errorf("operation failed: %w", ErrCapacityFull)
		assert.True(t, errors.Is(wrapped, ErrCapacityFull))
	})

	t.Run("double wrapped errors", func(t *testing.T) {
		inner := fmt.Errorf("inner: %w", ErrPermitNotHeld)
		outer := fmt.Errorf("outer: %w", inner)
		assert.True(t, errors.Is(outer, ErrPermitNotHeld))
	})
}

func TestSemaphoreTypeConstants(t *testing.T) {
	assert.Equal(t, "distributed", SemaphoreTypeDistributed)
	assert.Equal(t, "local", SemaphoreTypeLocal)
}

func TestScriptStatusConstants(t *testing.T) {
	assert.Equal(t, 0, scriptStatusOK)
	assert.Equal(t, 1, scriptStatusCapacityFull)
	assert.Equal(t, 2, scriptStatusTenantQuotaExceeded)
	assert.Equal(t, 3, scriptStatusNotHeld)
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "permit not held",
			err:      ErrPermitNotHeld,
			expected: ErrClassPermitNotHeld,
		},
		{
			name:     "wrapped permit not held",
			err:      fmt.Errorf("failed: %w", ErrPermitNotHeld),
			expected: ErrClassPermitNotHeld,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrClassTimeout,
		},
		{
			name:     "wrapped deadline exceeded",
			err:      fmt.Errorf("operation failed: %w", context.DeadlineExceeded),
			expected: ErrClassTimeout,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: ErrClassCanceled,
		},
		{
			name:     "wrapped context canceled",
			err:      fmt.Errorf("aborted: %w", context.Canceled),
			expected: ErrClassCanceled,
		},
		{
			name: "network error - redis unavailable",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: syscall.ECONNREFUSED,
			},
			expected: ErrClassRedisUnavailable,
		},
		{
			name:     "redis unavailable sentinel",
			err:      ErrRedisUnavailable,
			expected: ErrClassRedisUnavailable,
		},
		{
			name:     "other error - internal",
			err:      errors.New("unknown error"),
			expected: ErrClassInternal,
		},
		{
			name:     "capacity full - internal",
			err:      ErrCapacityFull,
			expected: ErrClassInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRedisError_ExcludesContextErrors(t *testing.T) {
	// context.Canceled 不应被视为 Redis 错误
	t.Run("context.Canceled is not redis error", func(t *testing.T) {
		assert.False(t, IsRedisError(context.Canceled))
	})

	// context.DeadlineExceeded 不应被视为 Redis 错误
	t.Run("context.DeadlineExceeded is not redis error", func(t *testing.T) {
		assert.False(t, IsRedisError(context.DeadlineExceeded))
	})

	// 包装的 context 错误也不应被视为 Redis 错误
	t.Run("wrapped context.Canceled is not redis error", func(t *testing.T) {
		wrapped := fmt.Errorf("operation failed: %w", context.Canceled)
		assert.False(t, IsRedisError(wrapped))
	})

	t.Run("wrapped context.DeadlineExceeded is not redis error", func(t *testing.T) {
		wrapped := fmt.Errorf("operation timeout: %w", context.DeadlineExceeded)
		assert.False(t, IsRedisError(wrapped))
	})
}

func TestIsRedisError_ClusterErrors(t *testing.T) {
	// Redis Cluster 相关错误应触发降级
	t.Run("CROSSSLOT error is redis error", func(t *testing.T) {
		assert.True(t, IsRedisError(redis.ErrCrossSlot))
	})

	t.Run("wrapped CROSSSLOT error is redis error", func(t *testing.T) {
		wrapped := fmt.Errorf("script failed: %w", redis.ErrCrossSlot)
		assert.True(t, IsRedisError(wrapped))
	})
}

func TestIsRedisClusterError(t *testing.T) {
	// 测试 isRedisClusterError 函数的覆盖
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isRedisClusterError(nil))
	})

	t.Run("regular error is not cluster error", func(t *testing.T) {
		assert.False(t, isRedisClusterError(errors.New("some error")))
	})

	t.Run("CROSSSLOT is cluster error", func(t *testing.T) {
		assert.True(t, isRedisClusterError(redis.ErrCrossSlot))
	})
}

// =============================================================================
// 代理 Lua 错误检测测试（P1 修复验证）
// =============================================================================

func TestIsRedisError_ProxyLuaErrors(t *testing.T) {
	// 测试代理不支持 Lua 脚本时的错误检测
	t.Run("Twemproxy unknown command eval", func(t *testing.T) {
		// Twemproxy 返回的错误格式
		err := errors.New("ERR unknown command 'eval'")
		assert.True(t, IsRedisError(err), "should detect Twemproxy eval error")
	})

	t.Run("Twemproxy unknown command evalsha", func(t *testing.T) {
		err := errors.New("ERR unknown command 'evalsha'")
		assert.True(t, IsRedisError(err), "should detect Twemproxy evalsha error")
	})

	t.Run("NOSCRIPT error reaching application layer", func(t *testing.T) {
		// 如果 NOSCRIPT 传到应用层，说明 EVAL 也失败了
		err := errors.New("NOSCRIPT No matching script. Please use EVAL")
		assert.True(t, IsRedisError(err), "should detect NOSCRIPT error")
	})

	t.Run("cluster support disabled", func(t *testing.T) {
		err := errors.New("ERR This instance has cluster support disabled")
		assert.True(t, IsRedisError(err), "should detect cluster support disabled error")
	})

	t.Run("wrapped proxy error", func(t *testing.T) {
		innerErr := errors.New("ERR unknown command 'eval'")
		wrappedErr := fmt.Errorf("script execution failed: %w", innerErr)
		assert.True(t, IsRedisError(wrappedErr), "should detect wrapped proxy error")
	})

	t.Run("normal redis error is not proxy error", func(t *testing.T) {
		// 普通的 Redis 错误不应该被误判为代理错误
		err := errors.New("ERR wrong number of arguments for 'get' command")
		assert.False(t, IsRedisError(err), "should not detect normal Redis argument error")
	})
}

func TestIsRedisProtocolError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "unknown command eval",
			err:      errors.New("ERR unknown command 'eval'"),
			expected: true,
		},
		{
			name:     "unknown command evalsha",
			err:      errors.New("ERR unknown command 'evalsha'"),
			expected: true,
		},
		{
			name:     "unknown command mixed case",
			err:      errors.New("ERR unknown command 'EVAL'"),
			expected: true,
		},
		{
			name:     "NOSCRIPT error",
			err:      errors.New("NOSCRIPT No matching script"),
			expected: true,
		},
		{
			name:     "cluster support disabled",
			err:      errors.New("ERR This instance has cluster support disabled"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "wrapped unknown command",
			err:      fmt.Errorf("redis: %s", "ERR unknown command 'eval'"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRedisProtocolError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
