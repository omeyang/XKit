// Package xsampling 提供通用的采样策略库。
//
// xsampling 遵循策略模式设计，提供统一的 Sampler 接口和多种采样策略实现，
// 适用于日志采样、链路追踪采样、指标采样等场景。
//
// # 核心接口
//
// Sampler 是采样策略的核心接口，ShouldSample(ctx) 返回是否采样。
//
// # 基础策略
//
// 包提供以下基础采样策略：
//
//   - Always(): 全采样，总是返回 true
//   - Never(): 不采样，总是返回 false
//   - NewRateSampler(rate): 固定比率采样（如 10% 采样率）
//   - NewCountSampler(n): 计数采样（每 n 个采样 1 个）
//   - NewProbabilitySampler(p): 概率采样
//
// # 高级策略
//
//   - NewCompositeSampler(mode, ...): 组合多个采样器（AND/OR 逻辑）
//   - NewKeyBasedSampler(rate, keyFunc): 基于 key 的一致性采样（使用 xxhash）
//
// # KeyBasedSampler 与跨进程一致性
//
// KeyBasedSampler 使用 xxhash（github.com/cespare/xxhash/v2）作为哈希算法，
// 这是一个确定性哈希算法，同一 key 在所有进程中产生相同的哈希值。
//
// 这对分布式追踪采样至关重要：
//   - 同一 trace_id 在所有服务中被一致地采样或丢弃
//   - 不同服务实例之间的采样决策保持一致
//   - 服务重启后采样行为不变
//
// 选择 xxhash 的原因：
//   - 确定性：相同输入总是产生相同输出（跨进程一致）
//   - 高性能：具体表现依赖环境与数据分布，以基准测试为准
//   - 分布均匀：采样偏差需通过基准/统计测试验证
//   - 零分配：热路径无内存分配
//   - 行业标准：Prometheus、OpenTelemetry 等项目广泛使用
//
// # 使用方式
//
// 调用 NewXxxSampler 创建采样器，通过 ShouldSample(ctx) 进行采样决策。
// 使用 All()/Any() 组合多个采样器（AND/OR 逻辑）。
//
// # 并发安全
//
// 所有采样器都是并发安全的，可以在多个 goroutine 中同时使用。
//
// # 性能
//
// 采样决策经过优化，热路径零内存分配。使用 math/rand/v2（Go 1.22+）
// 作为随机数源，单次采样决策耗时约 10ns，相比 crypto/rand 提升约 10 倍。
// 对于采样场景，统计随机性完全足够，无需密码学安全随机数。
//
// 基准结果（示例环境：linux/amd64, Xeon E5-2630 v4, `go test -bench=. -benchmem ./pkg/xsampling`）：
//   - BenchmarkKeyBasedSampler: ~29.7 ns/op, 0 allocs/op
//   - BenchmarkXXHash: ~71.6 ns/op
//   - BenchmarkMaphashString: ~85.1 ns/op
//   - BenchmarkFNVStdlib: ~122.5 ns/op
package xsampling
