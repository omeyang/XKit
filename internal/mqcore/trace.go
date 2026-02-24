package mqcore

import (
	"context"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// MergeTraceContext 合并两个 context 中的追踪信息。
// base 是基础 context，extracted 是从消息头提取的 context。
// 返回合并后的 context，优先使用 extracted 中的追踪信息。
//
// 合并字段包括 TraceID、SpanID、RequestID 和 TraceFlags，
// 与 syncTraceToXctx 保持一致，确保采样决策在 MQ 消费链路中正确传播。
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
