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
package xtrace
