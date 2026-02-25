package mqcore

import (
	"context"

	"github.com/omeyang/xkit/pkg/context/xctx"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// MergeTraceContext 合并两个 context 中的追踪信息。
// base 是基础 context，extracted 是从消息头提取的 context。
// 返回合并后的 context，优先使用 extracted 中的追踪信息。
//
// 合并字段包括 TraceID、SpanID、RequestID、TraceFlags（xctx 层）以及
// OTel SpanContext（若 extracted 包含有效的 OTel SpanContext，一并传递到 base，
// 确保 OTel 原生父子链在 MQ 消费端不断裂）。
//
// nil 参数会被视为 context.Background()，确保返回值始终非 nil。
func MergeTraceContext(base context.Context, extracted context.Context) context.Context {
	if base == nil {
		base = context.Background()
	}
	if extracted == nil {
		return base
	}

	base = mergeTraceField(base, xctx.TraceID(extracted), xctx.WithTraceID)
	base = mergeTraceField(base, xctx.SpanID(extracted), xctx.WithSpanID)
	base = mergeTraceField(base, xctx.RequestID(extracted), xctx.WithRequestID)
	base = mergeTraceField(base, xctx.TraceFlags(extracted), xctx.WithTraceFlags)

	// 保留 extracted 中的 OTel SpanContext，确保下游 trace.SpanContextFromContext
	// 能获取有效的父 Span 信息，避免被当作新根 Span 导致链路断裂。
	if sc := oteltrace.SpanContextFromContext(extracted); sc.IsValid() {
		base = oteltrace.ContextWithSpanContext(base, sc)
	}

	return base
}

// mergeTraceField 将单个追踪字段从 extracted 合并到 base context。
// value 为空时跳过，setter 失败时保留原 context（优雅降级）。
func mergeTraceField(
	base context.Context,
	value string,
	setter func(context.Context, string) (context.Context, error),
) context.Context {
	if value == "" {
		return base
	}
	if newCtx, err := setter(base, value); err == nil {
		return newCtx
	}
	return base
}
