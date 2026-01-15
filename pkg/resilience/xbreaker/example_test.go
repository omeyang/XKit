package xbreaker_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xbreaker"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// ExampleNewBreaker 演示基本的熔断器创建和使用
func ExampleNewBreaker() {
	// 创建熔断器，5次连续失败后熔断
	breaker := xbreaker.NewBreaker("my-service",
		xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
		xbreaker.WithTimeout(30*time.Second),
	)

	ctx := context.Background()

	// 执行受保护的操作
	err := breaker.Do(ctx, func() error {
		// 调用远程服务
		return nil
	})

	if err != nil {
		if xbreaker.IsOpen(err) {
			fmt.Println("熔断器已打开，请稍后重试")
		} else {
			fmt.Println("操作失败:", err)
		}
		return
	}

	fmt.Println("操作成功")
	// Output: 操作成功
}

// ExampleExecute 演示泛型执行函数
func ExampleExecute() {
	breaker := xbreaker.NewBreaker("user-service")
	ctx := context.Background()

	// 使用泛型函数执行带返回值的操作
	result, err := xbreaker.Execute(ctx, breaker, func() (string, error) {
		return "hello, world", nil
	})

	if err != nil {
		fmt.Println("错误:", err)
		return
	}

	fmt.Println(result)
	// Output: hello, world
}

// ExampleNewConsecutiveFailures 演示连续失败策略
func ExampleNewConsecutiveFailures() {
	// 3次连续失败后熔断
	policy := xbreaker.NewConsecutiveFailures(3)
	breaker := xbreaker.NewBreaker("api-service",
		xbreaker.WithTripPolicy(policy),
		xbreaker.WithTimeout(10*time.Second),
	)

	fmt.Println("熔断阈值:", policy.Threshold())
	fmt.Println("初始状态:", breaker.State())
	// Output:
	// 熔断阈值: 3
	// 初始状态: closed
}

// ExampleNewFailureRatio 演示失败率策略
func ExampleNewFailureRatio() {
	// 失败率超过50%且至少有10次请求时熔断
	policy := xbreaker.NewFailureRatio(0.5, 10)
	breaker := xbreaker.NewBreaker("payment-service",
		xbreaker.WithTripPolicy(policy),
	)

	fmt.Println("失败率阈值:", policy.Ratio())
	fmt.Println("最小请求数:", policy.MinRequests())
	fmt.Println("初始状态:", breaker.State())
	// Output:
	// 失败率阈值: 0.5
	// 最小请求数: 10
	// 初始状态: closed
}

// ExampleNewCompositePolicy 演示组合策略
func ExampleNewCompositePolicy() {
	// 组合多个策略：任一条件满足即熔断
	policy := xbreaker.NewCompositePolicy(
		xbreaker.NewConsecutiveFailures(5), // 连续失败5次
		xbreaker.NewFailureRatio(0.5, 20),  // 或失败率超过50%
		xbreaker.NewFailureCount(100),      // 或总失败数超过100
	)

	breaker := xbreaker.NewBreaker("critical-service",
		xbreaker.WithTripPolicy(policy),
	)

	fmt.Println("策略数量:", len(policy.Policies()))
	fmt.Println("初始状态:", breaker.State())
	// Output:
	// 策略数量: 3
	// 初始状态: closed
}

// ExampleWithOnStateChange 演示状态变化回调
func ExampleWithOnStateChange() {
	breaker := xbreaker.NewBreaker("monitored-service",
		xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(1)),
		xbreaker.WithOnStateChange(func(name string, from, to xbreaker.State) {
			fmt.Printf("熔断器 %s: %s -> %s\n", name, from, to)
		}),
	)

	ctx := context.Background()

	// 触发一次失败，导致熔断
	_ = breaker.Do(ctx, func() error {
		return errors.New("service unavailable")
	})

	// Output: 熔断器 monitored-service: closed -> open
}

// ExampleNewBreakerRetryer 演示熔断器+重试组合
func ExampleNewBreakerRetryer() {
	// 创建熔断器
	breaker := xbreaker.NewBreaker("remote-api",
		xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
	)

	// 创建重试器
	retryer := xretry.NewRetryer(
		xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		xretry.WithBackoffPolicy(xretry.NewExponentialBackoff()),
	)

	// 组合熔断器和重试器
	combo := xbreaker.NewBreakerRetryer(breaker, retryer)
	ctx := context.Background()

	var attempts int
	err := combo.DoWithRetry(ctx, func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		fmt.Println("最终失败:", err)
	} else {
		fmt.Println("成功，尝试次数:", attempts)
	}
	// Output: 成功，尝试次数: 2
}

// ExampleExecuteWithRetry 演示带返回值的熔断+重试
func ExampleExecuteWithRetry() {
	breaker := xbreaker.NewBreaker("data-service")
	retryer := xretry.NewRetryer(
		xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
	)
	combo := xbreaker.NewBreakerRetryer(breaker, retryer)
	ctx := context.Background()

	result, err := xbreaker.ExecuteWithRetry(ctx, combo, func() (int, error) {
		return 42, nil
	})

	if err != nil {
		fmt.Println("错误:", err)
	} else {
		fmt.Println("结果:", result)
	}
	// Output: 结果: 42
}

// ExampleNewRetryThenBreak 演示先重试后熔断模式
func ExampleNewRetryThenBreak() {
	// 先重试后熔断：重试期间的失败不影响熔断器计数
	retryer := xretry.NewRetryer(
		xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
	)
	breaker := xbreaker.NewBreaker("external-api",
		xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(2)),
	)

	rtb := xbreaker.NewRetryThenBreak(retryer, breaker)
	ctx := context.Background()

	// 第一次调用：重试3次都失败 -> 熔断器记录1次失败
	_ = rtb.Do(ctx, func(_ context.Context) error {
		return errors.New("always fail")
	})

	// 注意：使用 rtb.State() 和 rtb.Counts() 获取状态
	// 传入的 breaker 仅用于配置，状态由 rtb 内部维护
	fmt.Println("第一次调用后状态:", rtb.State())
	fmt.Println("总失败数:", rtb.Counts().TotalFailures)
	// Output:
	// 第一次调用后状态: closed
	// 总失败数: 1
}

// ExampleNewManagedBreaker 演示类型化托管熔断器
func ExampleNewManagedBreaker() {
	// 创建基础熔断器
	breaker := xbreaker.NewBreaker("typed-service")

	// 包装为特定类型的托管熔断器
	managed := xbreaker.NewManagedBreaker[string](breaker)

	// 直接执行，无需传入 context
	result, err := managed.Execute(func() (string, error) {
		return "typed result", nil
	})

	if err != nil {
		fmt.Println("错误:", err)
	} else {
		fmt.Println("结果:", result)
	}
	// Output: 结果: typed result
}

// ExampleNewCircuitBreaker 演示直接使用底层 gobreaker
func ExampleNewCircuitBreaker() {
	// 直接使用 gobreaker，获得完全控制
	cb := xbreaker.NewCircuitBreaker[string](xbreaker.Settings{
		Name:        "direct-breaker",
		MaxRequests: 3,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts xbreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})

	result, err := cb.Execute(func() (string, error) {
		return "direct result", nil
	})

	if err != nil {
		fmt.Println("错误:", err)
	} else {
		fmt.Println("结果:", result)
	}
	// Output: 结果: direct result
}

// ExampleIsOpen 演示错误判断
func ExampleIsOpen() {
	breaker := xbreaker.NewBreaker("test-service",
		xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(1)),
		xbreaker.WithTimeout(time.Hour),
	)
	ctx := context.Background()

	// 触发熔断
	_ = breaker.Do(ctx, func() error {
		return errors.New("failure")
	})

	// 下次调用时熔断器已打开
	err := breaker.Do(ctx, func() error {
		return nil
	})

	if xbreaker.IsOpen(err) {
		fmt.Println("熔断器已打开")
	}
	if xbreaker.IsBreakerError(err) {
		fmt.Println("这是熔断器错误")
	}
	if xbreaker.IsRecoverable(err) {
		fmt.Println("错误可恢复，稍后重试")
	}
	// Output:
	// 熔断器已打开
	// 这是熔断器错误
	// 错误可恢复，稍后重试
}

// ExampleBreaker_Counts 演示获取熔断器计数
func ExampleBreaker_Counts() {
	breaker := xbreaker.NewBreaker("stats-service")
	ctx := context.Background()

	// 执行一些操作
	_ = breaker.Do(ctx, func() error { return nil })
	_ = breaker.Do(ctx, func() error { return nil })
	_ = breaker.Do(ctx, func() error { return errors.New("fail") })

	counts := breaker.Counts()
	fmt.Println("总请求数:", counts.Requests)
	fmt.Println("成功数:", counts.TotalSuccesses)
	fmt.Println("失败数:", counts.TotalFailures)
	// Output:
	// 总请求数: 3
	// 成功数: 2
	// 失败数: 1
}
