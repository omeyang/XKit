// Package xlog 基于 log/slog 的结构化日志库。
//
// # 核心功能
//
//   - Builder 模式配置（输出目标、级别、格式、轮转）
//   - 自动从 context 注入 trace_id、tenant_id 等（EnrichHandler，默认启用）
//   - 动态级别调整（运行时热更新）
//   - 部署类型固定属性
//   - 全局 Logger 便利函数
//   - 延迟求值（Lazy* 系列函数）
//
// # 创建 Logger
//
// 使用 Builder 模式（first-error-wins：遇到第一个配置错误后，后续 Set 操作被跳过）。
// Builder 为一次性使用：调用 [Builder.Build] 后不可复用，需通过 [New] 创建新实例。
// Builder 方法：SetLevel、SetFormat、SetOutput、SetRotation、SetEnrich、
// SetDeploymentType、SetOnError、SetReplaceAttr。
//
// [SetReplaceAttr] 支持日志治理场景（字段重命名、敏感信息脱敏、字段过滤）。
// xlog 提供机制而非策略——无内置敏感字段黑名单，由调用方按业务需求配置脱敏规则。
//
// # 全局 Logger
//
// 适用于脚手架、小工具等简单场景，服务端推荐依赖注入。
//
//   - [Default]: 获取全局 Logger（惰性初始化：stderr、Info 级别、text 格式）
//   - [SetDefault]: 替换全局 Logger（nil 会被忽略）
//   - [ResetDefault]: 重置为未初始化状态（仅用于测试）
//   - [Debug]、[Info]、[Warn]、[Error]: 全局便利函数，签名为 (ctx, msg, ...slog.Attr)
//   - [Stack]: 全局便利函数，记录带堆栈的错误日志
//
// # 日志级别
//
// LevelDebug(-4)、LevelInfo(0)、LevelWarn(4)、LevelError(8)。
// 可通过 [ParseLevel] 从字符串解析。Level 实现 encoding.TextMarshaler/TextUnmarshaler，
// 支持配置文件直接序列化/反序列化。
//
// # 延迟求值
//
// 避免日志级别禁用时执行昂贵计算（注意：接口装箱的 1 次分配仍然存在）：
// [Lazy]、[LazyString]、[LazyInt]、[LazyError]、[LazyErr]、[LazyDuration]、[LazyGroup]。
//
// # 便捷属性
//
// [Err]、[Duration]、[Component]、[Operation]、[Count]、[UserID]、[StatusCode]、[Method]、[Path]。
//
// [Duration] 输出人类可读格式（如 "5s"、"1m30s"）。如需机器解析的数值格式，
// 使用 slog.Int64("duration_ms", d.Milliseconds())。
//
// # 派生 Logger 与级别控制
//
// [Logger.With] 和 [Logger.WithGroup] 返回 [Logger] 接口（不含 [Leveler]）。
// 底层实现同时实现了 [LoggerWithLevel]，可通过类型断言获取级别控制能力：
//
//	child := logger.With(slog.String("k", "v"))
//	if lwl, ok := child.(xlog.LoggerWithLevel); ok {
//	    lwl.SetLevel(xlog.LevelDebug)
//	}
//
// 派生 logger 共享父级的 LevelVar，动态级别变更会同步生效。
//
// # EnrichHandler 注意事项
//
// 当对启用了 enrich 的 logger 调用 WithGroup 时，trace_id、tenant_id 等注入字段
// 会被归入 group 下（slog handler 架构的固有限制）。如需 enrich 字段保持在顶层，
// 避免对启用 enrich 的 logger 调用 WithGroup。
package xlog
