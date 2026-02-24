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
//   - NewRateSampler(rate): 固定比率采样（如 10% 采样率），rate 超出 [0, 1] 范围返回错误
//   - NewCountSampler(n): 计数采样（每 n 个采样 1 个），n < 1 时返回错误
//
// # 高级策略
//
//   - NewCompositeSampler(mode, ...): 组合多个采样器（AND/OR 逻辑，短路求值），非法 mode 或 nil 子采样器返回错误。
//     短路求值意味着有状态子采样器（如 CountSampler）的内部状态仅在实际被求值时更新，
//     子采样器的排列顺序可能影响行为
//   - NewKeyBasedSampler(rate, keyFunc, opts...): 基于 key 的一致性采样（使用 xxhash），keyFunc 不能为 nil。
//     可选 WithOnEmptyKey 回调用于监控空 key 事件
//
// # 错误处理
//
// 所有构造函数对无效参数返回错误（fail-fast）：
//   - ErrInvalidRate: rate 超出 [0.0, 1.0] 范围或为 NaN
//   - ErrNilKeyFunc: keyFunc 为 nil
//   - ErrInvalidCount: count n < 1
//   - ErrInvalidMode: CompositeMode 不是 ModeAND 或 ModeOR
//   - ErrNilSampler: CompositeSampler 的子采样器为 nil
//
// # 不可变性与状态
//
// 所有采样器的配置（rate、n、mode 等）创建后不可变，不支持运行时动态修改。
// CountSampler 和 CompositeSampler 的内部计数器状态可通过 Reset() 重置。
// 如需动态调整采样率，建议使用 atomic.Pointer[Sampler] 持有采样器引用，
// 在配置变更时创建新采样器并原子替换。
//
// # 零值行为
//
// 所有采样器的结构体字段均未导出，应始终通过构造函数创建。零值行为仅作为安全兜底：
//   - CountSampler 零值：按全采样处理（避免除零 panic）
//   - RateSampler 零值：等同于 Never()（rate=0，不采样）
//   - CompositeSampler 零值：mode=ModeAND + 空列表 → 返回 true（AND 恒等元，等同于全采样）
//   - KeyBasedSampler 零值：rate=0 → 不采样（但 keyFunc 为 nil 会 panic，请勿使用零值）
//
// # 与 OTel 的关系
//
// xsampling.Sampler 是通用采样接口（ShouldSample(ctx) bool），
// 与 OTel trace.Sampler（ShouldSample(SamplingParameters) SamplingResult）
// 签名不同。xsampling 适用于日志、指标等通用采样场景；如需作为 OTel
// TracerProvider 的采样器，需自行编写适配层将 xsampling.Sampler 包装为
// trace.Sampler。本包不引入 OTel SDK 依赖以保持轻量。
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
// 当 KeyFunc 返回空字符串时（例如 context 中缺少 trace ID），采样器回退到随机采样。
// 此时仍保持近似的采样率语义，但失去跨进程一致性保证。
// 空 key 通常意味着上下文传播链路断裂，建议通过 WithOnEmptyKey 注册回调进行监控。
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
// 采样决策经过优化，热路径零内存分配。使用 crypto/rand 作为随机数源，
// 确保安全随机性，单次采样决策耗时约 50-100ns。对于采样场景（通常每请求
// 调用一次），此性能开销完全可接受。
//
// 运行基准测试：
//
//	go test -bench=. -benchmem ./pkg/observability/xsampling
package xsampling
