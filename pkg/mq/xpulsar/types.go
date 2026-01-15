package xpulsar

import "github.com/omeyang/xkit/internal/mqcore"

// Tracer 定义链路追踪器接口。
// 用于在消息发送时注入追踪上下文，在接收时提取追踪上下文。
type Tracer = mqcore.Tracer

// NoopTracer 是一个空实现的追踪器。
// 不执行任何操作，用于禁用追踪或作为默认值。
type NoopTracer = mqcore.NoopTracer

// OTelTracer 是基于 OpenTelemetry 的追踪器实现。
type OTelTracer = mqcore.OTelTracer

// NewOTelTracer 创建基于 OpenTelemetry 的追踪器。
var NewOTelTracer = mqcore.NewOTelTracer

// MergeTraceContext 合并追踪上下文。
// 用于将从消息中提取的追踪上下文合并到当前上下文。
var MergeTraceContext = mqcore.MergeTraceContext
