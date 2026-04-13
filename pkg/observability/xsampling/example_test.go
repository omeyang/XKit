package xsampling_test

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xsampling"
)

// exampleContextKey 示例用的 context key 类型
type exampleContextKey string

const traceIDKey exampleContextKey = "trace_id"

func ExampleNewRateSampler() {
	// 创建 10% 采样率的采样器
	sampler, err := xsampling.NewRateSampler(0.1)
	if err != nil {
		log.Fatal(err)
	}

	// 获取采样率
	fmt.Printf("Sampling rate: %.1f\n", sampler.Rate())
	// Output: Sampling rate: 0.1
}

func ExampleNewCountSampler() {
	// 创建每 5 个采样 1 个的采样器
	sampler, err := xsampling.NewCountSampler(5)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// 记录哪些被采样
	results := make([]bool, 10)
	for i := range 10 {
		results[i] = sampler.ShouldSample(ctx)
	}

	fmt.Println(results)
	// Output: [true false false false false true false false false false]
}

func ExampleAlways() {
	sampler := xsampling.Always()
	ctx := context.Background()

	// 全采样，总是返回 true
	allTrue := true
	for range 100 {
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
	for range 100 {
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
	counter, err := xsampling.NewCountSampler(2)
	if err != nil {
		log.Fatal(err)
	}
	sampler, err := xsampling.NewCompositeSampler(
		xsampling.ModeAND,
		xsampling.Always(), // 条件1：全采样
		counter,            // 条件2：每 2 个采样 1 个
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	results := make([]bool, 6)
	for i := range 6 {
		results[i] = sampler.ShouldSample(ctx)
	}

	fmt.Println(results)
	// Output: [true false true false true false]
}

func ExampleAll() {
	// All 是 NewCompositeSampler(ModeAND, ...) 的便捷写法
	sampler, err := xsampling.All(
		xsampling.Always(),
		xsampling.Always(),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

func ExampleAny() {
	// Any 是 NewCompositeSampler(ModeOR, ...) 的便捷写法
	sampler, err := xsampling.Any(
		xsampling.Never(),
		xsampling.Always(),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

func ExampleNewKeyBasedSampler() {
	// 创建基于 key 的一致性采样器
	sampler, err := xsampling.NewKeyBasedSampler(0.5, func(ctx context.Context) string {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			return v
		}
		return ""
	})
	if err != nil {
		log.Fatal(err)
	}

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
	sampler, err := xsampling.NewCountSampler(3)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// 前 3 次调用
	for range 3 {
		sampler.ShouldSample(ctx)
	}

	// 重置计数器
	sampler.Reset()

	// 重置后第一次调用返回 true
	fmt.Println(sampler.ShouldSample(ctx))
	// Output: true
}

func ExampleWithOnEmptyKey() {
	// 监控空 key 事件，帮助排查上下文传播问题
	var emptyKeyCount atomic.Int64
	sampler, err := xsampling.NewKeyBasedSampler(0.5, func(ctx context.Context) string {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			return v
		}
		return ""
	}, xsampling.WithOnEmptyKey(func() {
		emptyKeyCount.Add(1)
	}))
	if err != nil {
		log.Fatal(err)
	}

	// 空 key 场景（context 中没有 trace_id）
	sampler.ShouldSample(context.Background())
	fmt.Printf("Empty key events: %d\n", emptyKeyCount.Load())
	// Output: Empty key events: 1
}

// 演示日志采样场景
func Example_logSampling() {
	// 高 QPS 服务使用 1% 采样率
	sampler, err := xsampling.NewRateSampler(0.01)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	logCount := 0
	for range 10000 {
		if sampler.ShouldSample(ctx) {
			logCount++
		}
	}

	// 大约只有 1% 的日志被输出
	fmt.Printf("Logged approximately %d out of 10000 requests\n", logCount)
}

// 演示链路追踪采样场景
func Example_traceSampling() {
	// 按 trace_id 采样，确保同一条链路一致
	sampler, err := xsampling.NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(traceIDKey).(string); ok {
			return v
		}
		return ""
	})
	if err != nil {
		log.Fatal(err)
	}

	// 同一个 trace 的多个 span 应该被一致处理
	traceID := "trace-abc-123"
	ctx := context.WithValue(context.Background(), traceIDKey, traceID)

	// 模拟同一 trace 的多个 span
	results := make([]bool, 5)
	for i := range 5 {
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
