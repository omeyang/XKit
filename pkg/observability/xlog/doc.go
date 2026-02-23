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
// Builder 方法：SetLevel、SetFormat、SetOutput、SetRotation、SetEnrich、
// SetDeploymentType、SetOnError、SetReplaceAttr。
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
package xlog
