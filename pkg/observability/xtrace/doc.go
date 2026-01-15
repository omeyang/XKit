// Package xtrace 提供链路追踪信息的跨服务传播能力。
//
// # 设计理念
//
// xtrace 包负责 HTTP/gRPC 通信中链路追踪信息的提取和注入。
// 底层存储使用 xctx 包，xtrace 只做传输层适配，不维护状态。
//
// 支持以下追踪标识：
//   - TraceID: 链路追踪 ID（16字节，符合 W3C Trace Context）
//   - SpanID: 跨度 ID（8字节，符合 W3C Trace Context）
//   - RequestID: 请求 ID（业务层面，通常是 UUID）
//   - TraceFlags: 采样标志（1字节，W3C trace-flags，如 "01" 表示已采样）
//
// # 协议支持
//
// HTTP Header:
//   - X-Trace-ID: 链路追踪 ID
//   - X-Span-ID: 跨度 ID
//   - X-Request-ID: 请求 ID
//   - traceparent: W3C Trace Context 标准头
//   - tracestate: W3C Trace Context 扩展信息
//
// gRPC Metadata:
//   - x-trace-id: 链路追踪 ID
//   - x-span-id: 跨度 ID
//   - x-request-id: 请求 ID
//   - traceparent: W3C Trace Context 标准
//   - tracestate: W3C Trace Context 扩展信息
//
// # 使用方式
//
// HTTP：使用 HTTPMiddleware() 服务端中间件，InjectToRequest() 客户端注入。
// gRPC：使用 GRPCUnaryServerInterceptor() 服务端拦截器，
// GRPCUnaryClientInterceptor() 客户端拦截器。
//
// # W3C Trace Context
//
// 本包支持 W3C Trace Context 规范（https://www.w3.org/TR/trace-context/）。
// traceparent 格式：{version}-{trace-id}-{parent-id}-{trace-flags}
// 示例：00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
//
// 版本兼容性：
//   - 支持 W3C 前向兼容规则：未知版本（> "00"）按 version-00 格式解析
//   - 版本 "ff" 保留，始终视为无效
//   - 未来版本可能包含额外字段，解析时自动忽略
//
// 解析优先级：
//  1. 优先使用 traceparent 头（W3C 标准）
//  2. 回退到自定义 X-Trace-ID/X-Span-ID 头
//
// # Tracestate 处理说明
//
// tracestate 头用于厂商扩展信息（采样策略、路由提示等）。
// 当前设计：
//   - 解析：tracestate 会被解析到 TraceInfo.Tracestate 字段
//   - 存储：tracestate 不自动存入 context（需手动处理）
//   - 传播：InjectToRequest/InjectToOutgoingContext 不自动传播 tracestate
//   - 手动透传：可通过 InjectTraceToHeader/InjectTraceToMetadata 手动设置
//
// 设计理由：tracestate 内容与厂商相关，中间服务盲目传递可能导致问题。
// 如需完整 tracestate 支持，建议使用 OpenTelemetry SDK。
//
// # 自动生成行为（AutoGenerate）
//
// 默认情况下，HTTPMiddleware 和 GRPCUnaryServerInterceptor 会自动生成缺失的追踪 ID。
// 这种行为是逐字段独立处理的：
//
//   - 如果 TraceID 缺失但 SpanID 存在：仅生成 TraceID
//   - 如果 SpanID 缺失但 TraceID 存在：仅生成 SpanID
//   - 如果两者都缺失：分别生成新的 TraceID 和 SpanID
//
// 潜在影响：当上游只传递部分字段时（如仅传 TraceID），当前服务会生成新的 SpanID，
// 这可能导致链路图上出现"伪父子关系"——新生成的 SpanID 与上游的 SpanID 无关联。
//
// 如需严格控制自动生成行为，可使用选项禁用：
//
//	xtrace.HTTPMiddlewareWithOptions(xtrace.WithAutoGenerate(false))
//	xtrace.GRPCUnaryServerInterceptorWithOptions(xtrace.WithGRPCAutoGenerate(false))
//
// 禁用后，缺失的字段将保持为空，不会自动生成。
//
// # W3C traceparent 大小写处理
//
// W3C Trace Context 规范要求 trace-id、parent-id、trace-flags 必须是小写十六进制。
// 本包在生成 traceparent（InjectToRequest/InjectToOutgoingContext）时会自动转换为小写，
// 确保输出符合 W3C 规范，避免被严格实现拒绝。
//
// 解析时（ExtractFromHTTPHeader/ExtractFromMetadata）同时接受大写和小写输入，
// 保持向后兼容性。
//
// # trace-flags 格式校验
//
// trace-flags 必须是 2 位十六进制字符（如 "00"、"01"、"ff"）。
// 无效格式的 trace-flags 会被丢弃并记录警告日志，不会注入到 context 中。
// 这避免了无效值污染 context 并在传播时被静默替换为 "00" 导致的采样语义丢失。
package xtrace
