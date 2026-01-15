package mqcore

import (
	"context"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// MergeTraceContext 合并两个 context 中的追踪信息。
// base 是基础 context，extracted 是从消息头提取的 context。
// 返回合并后的 context，优先使用 extracted 中的追踪信息。
func MergeTraceContext(base context.Context, extracted context.Context) context.Context {
	if base == nil {
		return extracted
	}
	if extracted == nil {
		return base
	}

	if traceID := xctx.TraceID(extracted); traceID != "" {
		newCtx, err := xctx.WithTraceID(base, traceID)
		if err == nil {
			base = newCtx
		}
	}
	if spanID := xctx.SpanID(extracted); spanID != "" {
		newCtx, err := xctx.WithSpanID(base, spanID)
		if err == nil {
			base = newCtx
		}
	}
	if requestID := xctx.RequestID(extracted); requestID != "" {
		newCtx, err := xctx.WithRequestID(base, requestID)
		if err == nil {
			base = newCtx
		}
	}

	return base
}
