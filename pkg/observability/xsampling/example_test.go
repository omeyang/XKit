package xsampling_test

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/pkg/observability/xsampling"
)

// exampleContextKey 示例用的 context key 类型
type exampleContextKey string

const traceIDKey exampleContextKey = "trace_id"

func ExampleNewRateSampler() {
	// 创建 10% 采样率的采样器
	sampler := xsampling.NewRateSampler(0.1)

	// 获取采样率
	fmt.Printf("Sampling rate: %.1f\n", sampler.Rate())
	// Output: Sampling rate: 0.1
}

func ExampleNewCountSampler() {
	// 创建每 5 个采样 1 个的采样器
	sampler := xsampling.NewCountSampler(5)
	ctx := context.Background()

	// 记录哪些被采样
	results := make([]bool, 10)
	for i := 0; i < 10; i++ {
		results[i] = sampler.ShouldSample(ctx)
	}

	fmt.Println(results)
	// Output: [true false false false false true false false false false]
}

func ExampleNewProbabilitySampler() {
	// 创建 50% 概率采样器
	sampler := xsampling.NewProbabilitySampler(0.5)

	// 获取概率值
	fmt.Printf("Sampling probability: %.1f\n", sampler.Probability())
	// Output: Sampling probability: 0.5
}

func ExampleAlways() {
	sampler := xsampling.Always()
	ctx := context.Background()

	// 全采样，总是返回 true
	allTrue := true
	for i := 0; i < 100; i++ {
		if !sampler.ShouldSample(ctx) {
			allTrue = false
			break
		}
	}

	fmt.Printf("All samples: %v\n", allTrue)
	// Output: All samples: true
}

func ExampleNever() {
	sampler := xsampling.Never()
	ctx := context.Background()

	// 不采样，总是返回 false
	allFalse := true
	for i := 0; i < 100; i++ {
		if sampler.ShouldSample(ctx) {
			allFalse = false
			break
		}
	}

	fmt.Printf("No samples: %v\n", allFalse)
	// Output: No samples: true
}

func ExampleNewCompositeSampler() {
	// 创建组合采样器：需要同时满足两个条件
	sampler := xsampling.NewCompositeSampler(
		xsampling.ModeAND,
		xsampling.Always(),           // 条件1：全采样
		xsampling.NewCountSampler(2), // 条件2：每 2 个采样 1 个
	)
	ctx := context.Background()

	results := make([]bool, 6)
	for i := 0; i < 6; i++ {
		results[i] = sampler.ShouldSample(ctx)
	}

	fmt.Println(results)
	// Output: [true false true false true false]
}

func ExampleAll() {
	// All 是 NewCompositeSampler(ModeAND, ...) 的便捷写法
	sampler := xsampling.All(
		xsampling.Always(),
		xsampling.Always(),
	)
	ctx := context.Background()

	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

func ExampleAny() {
	// Any 是 NewCompositeSampler(ModeOR, ...) 的便捷写法
	sampler := xsampling.Any(
		xsampling.Never(),
		xsampling.Always(),
	)
	ctx := context.Background()

	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

func ExampleNewKeyBasedSampler() {
	// 创建基于 key 的一致性采样器
	sampler := xsampling.NewKeyBasedSampler(0.5, func(ctx context.Context) string {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			return v
		}
		return ""
	})

	// 相同的 key 总是产生相同的结果
	ctx1 := context.WithValue(context.Background(), traceIDKey, "abc123")
	ctx2 := context.WithValue(context.Background(), traceIDKey, "abc123")

	result1 := sampler.ShouldSample(ctx1)
	result2 := sampler.ShouldSample(ctx2)

	// 验证一致性：相同 key 产生相同结果
	fmt.Printf("Same key same result: %v\n", result1 == result2)
	fmt.Printf("Rate: %.1f\n", sampler.Rate())
	// Output:
	// Same key same result: true
	// Rate: 0.5
}

func ExampleCountSampler_Reset() {
	sampler := xsampling.NewCountSampler(3)
	ctx := context.Background()

	// 前 3 次调用
	for i := 0; i < 3; i++ {
		sampler.ShouldSample(ctx)
	}

	// 重置计数器
	sampler.Reset()

	// 重置后第一次调用返回 true
	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

// 演示日志采样场景
func Example_logSampling() {
	// 高 QPS 服务使用 1% 采样率
	sampler := xsampling.NewRateSampler(0.01)
	ctx := context.Background()

	logCount := 0
	for i := 0; i < 10000; i++ {
		if sampler.ShouldSample(ctx) {
			// 这里是实际的日志输出
			// log.Info("Request processed", "id", i)
			logCount++
		}
	}

	// 大约只有 1% 的日志被输出
	fmt.Printf("Logged approximately %d out of 10000 requests\n", logCount)
}

// 演示链路追踪采样场景
func Example_traceSampling() {
	// 按 trace_id 采样，确保同一条链路一致
	sampler := xsampling.NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			return v
		}
		return ""
	})

	// 同一个 trace 的多个 span 应该被一致处理
	traceID := "trace-abc-123"
	ctx := context.WithValue(context.Background(), traceIDKey, traceID)

	// 模拟同一 trace 的多个 span
	results := make([]bool, 5)
	for i := 0; i < 5; i++ {
		results[i] = sampler.ShouldSample(ctx)
	}

	// 所有结果应该相同
	allSame := true
	for _, r := range results[1:] {
		if r != results[0] {
			allSame = false
			break
		}
	}

	fmt.Printf("All spans in trace sampled consistently: %v\n", allSame)
	// Output: All spans in trace sampled consistently: true
}
