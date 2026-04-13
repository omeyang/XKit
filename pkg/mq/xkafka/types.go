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

// 设计决策: 使用 var 重导出 mqcore 函数，而非函数包装。
// var 重导出是 Go 中从 internal 包转发 API 的常见惯用法。
// 虽然 var 理论上可被外部赋值覆盖，但实际风险极低，
// 且改为函数包装需要直接 import go.opentelemetry.io/otel/propagation（影响依赖树）。

// NewOTelTracer 创建 OTelTracer。
var NewOTelTracer = mqcore.NewOTelTracer

// WithOTelPropagator 设置自定义的 Propagator。
var WithOTelPropagator = mqcore.WithOTelPropagator

// MergeTraceContext 合并两个 context 中的追踪信息。
var MergeTraceContext = mqcore.MergeTraceContext
