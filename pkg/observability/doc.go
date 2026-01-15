// Package observability 提供可观测性相关的子包。
//
// 子包列表：
//   - xlog: 结构化日志，基于 log/slog 扩展
//   - xtrace: HTTP/gRPC 链路追踪中间件
//   - xmetrics: 统一可观测性接口（指标、追踪、日志）
//   - xsampling: 采样策略
//   - xrotate: 日志文件轮转
//
// 设计原则：
//   - 遵循 OpenTelemetry 语义规范
//   - 自动从 context 中提取追踪信息注入日志
//   - 支持动态级别控制和采样策略
package observability
