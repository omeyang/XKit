package xbreaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBreaker_HalfOpen_Concurrent 验证 HalfOpen 状态下的并发行为：
// 只有 maxRequests 个请求能通过，其余返回 ErrTooManyRequests
func TestBreaker_HalfOpen_Concurrent(t *testing.T) {
	const maxRequests = 2
	b := NewBreaker("test",
		WithTripPolicy(NewConsecutiveFailures(1)),
		WithTimeout(50*time.Millisecond),
		WithMaxRequests(maxRequests),
	)
	ctx := context.Background()

	// 触发熔断
	_ = b.Do(ctx, func() error { return errTest })
	require.Equal(t, StateOpen, b.State())

	// 等待进入 HalfOpen
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, StateHalfOpen, b.State())

	// 并发发起多个请求
	const goroutines = 10
	results := make(chan error, goroutines)
	for range goroutines {
		go func() {
			results <- b.Do(ctx, func() error {
				time.Sleep(10 * time.Millisecond) // 模拟操作耗时
				return nil
			})
		}()
	}

	var passed, rejected int
	for range goroutines {
		err := <-results
		if err == nil {
			passed++
		} else if IsTooManyRequests(err) {
			rejected++
			// 验证 ErrTooManyRequests 被包装为不可重试的 BreakerError
			var be *BreakerError
			assert.True(t, errors.As(err, &be))
			assert.False(t, be.Retryable())
		}
	}

	// 至少有部分通过，其余被拒绝
	assert.LessOrEqual(t, passed, int(maxRequests), "should not exceed maxRequests")
	assert.Greater(t, rejected, 0, "some requests should be rejected")
	assert.Equal(t, goroutines, passed+rejected, "all goroutines should complete")
}

func TestExecute_NilBreaker(t *testing.T) {
	_, err := Execute(context.Background(), nil, func() (string, error) {
		return "hello", nil
	})
	assert.ErrorIs(t, err, ErrNilBreaker)
}

func TestWithExcludePolicy(t *testing.T) {
	// 自定义排除策略：context.Canceled 被排除在统计之外
	excludePolicy := &testExcludePolicy{
		excludedErrors: []error{context.Canceled},
	}

	t.Run("excluded error does not affect counts", func(t *testing.T) {
		b := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
			WithExcludePolicy(excludePolicy),
		)
		ctx := context.Background()

		// context.Canceled 应被排除，不计入失败
		_ = b.Do(ctx, func() error { return context.Canceled })
		_ = b.Do(ctx, func() error { return context.Canceled })
		_ = b.Do(ctx, func() error { return context.Canceled })

		// 熔断器仍为 Closed（排除的错误不影响计数）
		assert.Equal(t, StateClosed, b.State())
		// 注意：gobreaker 的 IsExcluded 回调使得排除的错误不计入任何计数
	})

	t.Run("non-excluded error still triggers trip", func(t *testing.T) {
		b := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
			WithExcludePolicy(excludePolicy),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()

		// 普通错误仍计入失败
		_ = b.Do(ctx, func() error { return errTest })
		_ = b.Do(ctx, func() error { return errTest })

		assert.Equal(t, StateOpen, b.State())
	})

	t.Run("with SuccessPolicy together", func(t *testing.T) {
		// 同时使用 ExcludePolicy 和 SuccessPolicy
		successPolicy := &customSuccessPolicy{
			successErrors: []error{errTest},
		}
		b := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
			WithExcludePolicy(excludePolicy),
			WithSuccessPolicy(successPolicy),
		)
		ctx := context.Background()

		// context.Canceled 被排除
		_ = b.Do(ctx, func() error { return context.Canceled })
		// errTest 被 SuccessPolicy 视为成功
		_ = b.Do(ctx, func() error { return errTest })

		assert.Equal(t, StateClosed, b.State())
	})

	t.Run("ExcludePolicy getter", func(t *testing.T) {
		b := NewBreaker("test", WithExcludePolicy(excludePolicy))
		assert.Equal(t, excludePolicy, b.ExcludePolicy())
	})

	t.Run("nil ExcludePolicy not set", func(t *testing.T) {
		b := NewBreaker("test", WithExcludePolicy(nil))
		assert.Nil(t, b.ExcludePolicy())
	})

	t.Run("IsExcluded method", func(t *testing.T) {
		b := NewBreaker("test", WithExcludePolicy(excludePolicy))
		assert.True(t, b.IsExcluded(context.Canceled))
		assert.False(t, b.IsExcluded(errTest))
		assert.False(t, b.IsExcluded(nil))
	})

	t.Run("IsExcluded without policy", func(t *testing.T) {
		b := NewBreaker("test")
		assert.False(t, b.IsExcluded(errTest))
		assert.False(t, b.IsExcluded(nil))
	})
}

// TestWithExcludePolicy_RatioPolicy 验证 FG-S1 修复：
// ExcludePolicy 与比率策略组合时，排除的请求不影响熔断判定的分母
func TestWithExcludePolicy_RatioPolicy(t *testing.T) {
	excludePolicy := &testExcludePolicy{
		excludedErrors: []error{context.Canceled},
	}

	t.Run("excluded requests should not inflate minRequests", func(t *testing.T) {
		// 场景：minRequests=4, ratio=0.5
		// 执行：3 excluded(Canceled) + 2 failures + 1 success = 6 total, 3 effective
		// 修复后：effective=3 < minRequests=4 → 不触发（有效请求不足）
		b := NewBreaker("test",
			WithTripPolicy(NewFailureRatio(0.5, 4)),
			WithExcludePolicy(excludePolicy),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()

		// 3 个被排除的请求
		for range 3 {
			_ = b.Do(ctx, func() error { return context.Canceled })
		}
		// 2 个失败
		_ = b.Do(ctx, func() error { return errTest })
		_ = b.Do(ctx, func() error { return errTest })
		// 1 个成功
		_ = b.Do(ctx, func() error { return nil })

		// 有效请求 = 3（不足 minRequests=4），不应触发熔断
		assert.Equal(t, StateClosed, b.State(),
			"should not trip: effective requests (3) < minRequests (4)")
	})

	t.Run("effective ratio triggers trip correctly", func(t *testing.T) {
		// 场景：minRequests=3, ratio=0.5
		// 执行：2 excluded + 2 failures + 1 success = 5 total, 3 effective
		// 有效失败率 = 2/3 ≈ 0.67 > 0.5 → 应触发
		b := NewBreaker("test",
			WithTripPolicy(NewFailureRatio(0.5, 3)),
			WithExcludePolicy(excludePolicy),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()

		// 2 个被排除的请求
		_ = b.Do(ctx, func() error { return context.Canceled })
		_ = b.Do(ctx, func() error { return context.Canceled })
		// 1 个成功
		_ = b.Do(ctx, func() error { return nil })
		// 2 个失败
		_ = b.Do(ctx, func() error { return errTest })
		_ = b.Do(ctx, func() error { return errTest })

		assert.Equal(t, StateOpen, b.State(),
			"should trip: effective ratio = 2/3 ≈ 0.67 > 0.5")
	})
}

type testExcludePolicy struct {
	excludedErrors []error
}

func (p *testExcludePolicy) IsExcluded(err error) bool {
	for _, e := range p.excludedErrors {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// TestWithOnStateChange_PanicRecovery 验证 FG-S1 修复：
// 回调 panic 不应导致进程崩溃，应被捕获并记录
func TestWithOnStateChange_PanicRecovery(t *testing.T) {
	recovered := make(chan struct{})
	b := NewBreaker("test-panic",
		WithTripPolicy(NewConsecutiveFailures(1)),
		WithOnStateChange(func(name string, from, to State) {
			defer func() { close(recovered) }()
			panic("callback panic for testing")
		}),
	)

	ctx := context.Background()
	// 触发熔断，回调会 panic
	_ = b.Do(ctx, func() error { return errTest })

	// 等待 goroutine 中的 panic 被 recover
	select {
	case <-recovered:
		// 回调的 panic 被捕获，进程没有崩溃
	case <-time.After(time.Second):
		t.Fatal("OnStateChange panic recovery did not complete within timeout")
	}

	// 验证熔断器状态仍然正确
	assert.Equal(t, StateOpen, b.State())
}

// === FG-S1/L2/L3 修复验证：nil ctx / nil fn 直接测试 ===

func TestBreaker_Do_NilContext(t *testing.T) {
	b := NewBreaker("test")
	err := b.Do(nil, func() error { return nil }) //nolint:staticcheck // 测试 nil context 入口
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestBreaker_Do_NilFunc(t *testing.T) {
	b := NewBreaker("test")
	err := b.Do(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilFunc)
}

func TestExecute_NilContext(t *testing.T) {
	b := NewBreaker("test")
	_, err := Execute(nil, b, func() (string, error) { return "", nil }) //nolint:staticcheck // 测试 nil context 入口
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestExecute_NilFunc(t *testing.T) {
	b := NewBreaker("test")
	_, err := Execute[string](context.Background(), b, nil)
	assert.ErrorIs(t, err, ErrNilFunc)
}

func TestManagedBreaker_Execute_NilFunc(t *testing.T) {
	b := NewBreaker("test")
	m, err := NewManagedBreaker[string](b)
	require.NoError(t, err)

	_, err = m.Execute(nil)
	assert.ErrorIs(t, err, ErrNilFunc)
}

func TestWithOnStateChange_NilIgnored(t *testing.T) {
	called := make(chan struct{})
	b := NewBreaker("test",
		WithTripPolicy(NewConsecutiveFailures(1)),
		WithOnStateChange(func(name string, from, to State) {
			close(called)
		}),
		WithOnStateChange(nil), // nil 不应覆盖之前设置的回调
	)

	ctx := context.Background()
	_ = b.Do(ctx, func() error { return errTest })

	// 回调异步执行，等待完成
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("original callback should still be active after nil WithOnStateChange")
	}
}

// === FG-S1 修复验证：nil 接收者防护 ===

func TestBreaker_NilReceiver(t *testing.T) {
	t.Run("Do returns ErrNilBreaker", func(t *testing.T) {
		var b *Breaker
		err := b.Do(context.Background(), func() error { return nil })
		assert.ErrorIs(t, err, ErrNilBreaker)
	})

	t.Run("State returns StateClosed", func(t *testing.T) {
		var b *Breaker
		assert.Equal(t, StateClosed, b.State())
	})

	t.Run("Counts returns zero value", func(t *testing.T) {
		var b *Breaker
		counts := b.Counts()
		assert.Equal(t, uint32(0), counts.Requests)
	})
}

func TestBreaker_NilReceiver_IsSuccessful(t *testing.T) {
	var b *Breaker
	assert.True(t, b.IsSuccessful(nil))
	assert.False(t, b.IsSuccessful(errTest))
}

func TestBreaker_NilReceiver_IsExcluded(t *testing.T) {
	var b *Breaker
	assert.False(t, b.IsExcluded(nil))
	assert.False(t, b.IsExcluded(errTest))
}

func TestManagedBreaker_NilReceiver(t *testing.T) {
	t.Run("Execute returns ErrNilManagedBreaker", func(t *testing.T) {
		var m *ManagedBreaker[string]
		_, err := m.Execute(func() (string, error) { return "hello", nil })
		assert.ErrorIs(t, err, ErrNilManagedBreaker)
	})

	t.Run("State returns StateClosed", func(t *testing.T) {
		var m *ManagedBreaker[string]
		assert.Equal(t, StateClosed, m.State())
	})

	t.Run("Counts returns zero value", func(t *testing.T) {
		var m *ManagedBreaker[string]
		counts := m.Counts()
		assert.Equal(t, uint32(0), counts.Requests)
	})
}

// === nil BreakerOption 防护验证 ===

func TestNewBreaker_NilOption(t *testing.T) {
	t.Run("nil option in variadic is skipped", func(t *testing.T) {
		b := NewBreaker("test", nil, WithTimeout(30*time.Second), nil)
		assert.NotNil(t, b)
		assert.Equal(t, 30*time.Second, b.timeout)
	})

	t.Run("all nil options", func(t *testing.T) {
		b := NewBreaker("test", nil, nil)
		assert.NotNil(t, b)
		assert.Equal(t, DefaultTimeout, b.timeout)
	})
}

// === typed-nil 策略接口防护验证 ===

func TestWithTripPolicy_TypedNilIgnored(t *testing.T) {
	var p *ConsecutiveFailuresPolicy
	b := NewBreaker("test", WithTripPolicy(p))
	assert.NotNil(t, b.TripPolicy(), "typed-nil TripPolicy should be silently ignored, default retained")
}

func TestWithSuccessPolicy_TypedNilIgnored(t *testing.T) {
	var p *customSuccessPolicy
	b := NewBreaker("test", WithSuccessPolicy(p))
	assert.Nil(t, b.SuccessPolicy(), "typed-nil SuccessPolicy should be silently ignored")
}

func TestWithExcludePolicy_TypedNilIgnored(t *testing.T) {
	var p *testExcludePolicy
	b := NewBreaker("test", WithExcludePolicy(p))
	assert.Nil(t, b.ExcludePolicy(), "typed-nil ExcludePolicy should be silently ignored")
}

// === FG-M1 修复验证：业务函数返回 gobreaker sentinel 不被误归因 ===

func TestBreaker_Do_BusinessFuncReturnsSentinel(t *testing.T) {
	t.Run("business func returning ErrOpenState is not wrapped", func(t *testing.T) {
		b := NewBreaker("test",
			WithTripPolicy(NewNeverTrip()), // 永不熔断，确保 fn 被调用
		)
		ctx := context.Background()

		// 业务函数返回 gobreaker.ErrOpenState（不应被包装为 BreakerError）
		err := b.Do(ctx, func() error {
			return ErrOpenState
		})

		// 错误应原样返回，不被包装为 BreakerError
		assert.ErrorIs(t, err, ErrOpenState)
		var be *BreakerError
		assert.False(t, errors.As(err, &be),
			"business function's ErrOpenState should not be wrapped as BreakerError")
	})

	t.Run("actual breaker open is still wrapped", func(t *testing.T) {
		b := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()

		// 触发熔断
		_ = b.Do(ctx, func() error { return errTest })
		assert.Equal(t, StateOpen, b.State())

		// 熔断器拒绝的请求应该被包装为 BreakerError
		err := b.Do(ctx, func() error { return nil })
		var be *BreakerError
		assert.True(t, errors.As(err, &be),
			"breaker rejection should be wrapped as BreakerError")
		assert.Equal(t, StateOpen, be.State)
	})
}

// === FG-M3 修复验证：BreakerError.State 从错误类型推导 ===

func TestWrapBreakerError_StateDerivedFromError(t *testing.T) {
	t.Run("ErrOpenState maps to StateOpen", func(t *testing.T) {
		err := wrapBreakerError(ErrOpenState, "test")
		var be *BreakerError
		require.True(t, errors.As(err, &be))
		assert.Equal(t, StateOpen, be.State)
	})

	t.Run("ErrTooManyRequests maps to StateHalfOpen", func(t *testing.T) {
		err := wrapBreakerError(ErrTooManyRequests, "test")
		var be *BreakerError
		require.True(t, errors.As(err, &be))
		assert.Equal(t, StateHalfOpen, be.State)
	})
}
