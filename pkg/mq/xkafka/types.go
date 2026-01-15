package xkafka

import (
	"github.com/omeyang/xkit/internal/mqcore"
)

// Tracer 定义链路追踪接口。
// 用于在消息生产/消费时注入和提取追踪信息。
type Tracer = mqcore.Tracer

// NoopTracer 是 Tracer 的空实现。
// 当用户不需要链路追踪时使用。
type NoopTracer = mqcore.NoopTracer

// OTelTracer 基于 OpenTelemetry 的链路追踪实现。
type OTelTracer = mqcore.OTelTracer

// OTelTracerOption 定义 OTelTracer 的配置选项。
type OTelTracerOption = mqcore.OTelTracerOption

// NewOTelTracer 创建 OTelTracer。
var NewOTelTracer = mqcore.NewOTelTracer

// WithOTelPropagator 设置自定义的 Propagator。
var WithOTelPropagator = mqcore.WithOTelPropagator

// MergeTraceContext 合并两个 context 中的追踪信息。
var MergeTraceContext = mqcore.MergeTraceContext
