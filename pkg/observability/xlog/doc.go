// Package xlog 基于 log/slog 的结构化日志库。
//
// xlog 是对 Go 1.21+ 标准库 log/slog 的封装，提供：
//   - Builder 模式配置（输出目标、级别、格式、轮转）
//   - 自动从 context 注入追踪和身份信息（EnrichHandler）
//   - 动态级别调整（运行时热更新）
//   - 部署类型固定属性
//   - 全局 Logger 便利函数
//
// # 基本用法
//
// 使用 New().SetLevel(...).SetFormat(...).Build() 创建 Logger。
// Build() 返回 Logger、cleanup 函数和 error。
//
// # 主要功能
//
//   - SetRotation：启用日志轮转（基于 xrotate）
//   - SetEnrich：自动注入 trace_id、tenant_id 等（默认启用）
//   - SetDeploymentType：注入部署类型固定属性
//   - SetOnError：接收 Handler.Handle 失败通知
//   - SetReplaceAttr：字段规范化、脱敏
//
// # 日志级别
//
// 支持 LevelDebug(-4)、LevelInfo(0)、LevelWarn(4)、LevelError(8)。
// 可通过 ParseLevel 从字符串解析。
//
// # 全局 Logger
//
// 适用于脚手架、小工具、迁移期代码。服务端推荐依赖注入。
// 使用 xlog.Info/Error 等全局函数，或通过 SetDefault 替换。
//
// # 延迟求值
//
// 使用 Lazy* 函数避免不必要的计算：
// Lazy、LazyString、LazyInt、LazyError、LazyDuration、LazyGroup。
//
// # 便捷属性
//
// Err、Duration、Component、Operation、Count、UserID、StatusCode、Method、Path。
package xlog
