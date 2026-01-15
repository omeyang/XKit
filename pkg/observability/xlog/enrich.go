package xlog

import (
	"context"
	"log/slog"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// EnrichHandler 自动从 context 提取追踪和身份信息并注入日志
//
// 装饰模式实现，包装底层 slog.Handler，在 Handle() 时自动添加：
//   - trace: trace_id, span_id, request_id
//   - identity: platform_id, tenant_id, tenant_name
//
// Best-effort 策略：即使 context 中缺少某些字段，也不会影响日志记录。
type EnrichHandler struct {
	base slog.Handler
}

// NewEnrichHandler 创建 EnrichHandler
func NewEnrichHandler(base slog.Handler) *EnrichHandler {
	return &EnrichHandler{base: base}
}

// Enabled 委托给底层 handler
func (h *EnrichHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

// maxEnrichAttrs 最大注入属性数量（trace 3 + identity 3）
const maxEnrichAttrs = xctx.TraceFieldCount + xctx.IdentityFieldCount

// Handle 在调用底层 handler 前，从 context 提取追踪和身份信息
//
// 重要：根据 slog 契约，必须 Clone record 后再修改，避免影响其他 handler
//
// 性能优化：使用栈数组 [6]slog.Attr 避免热路径堆分配
func (h *EnrichHandler) Handle(ctx context.Context, r slog.Record) error {
	// 使用栈数组避免堆分配
	var buf [maxEnrichAttrs]slog.Attr
	attrs := buf[:0]
	attrs = xctx.AppendTraceAttrs(attrs, ctx)
	attrs = xctx.AppendIdentityAttrs(attrs, ctx)

	// 如果有属性需要添加，必须 Clone record
	if len(attrs) > 0 {
		// Clone record 后再修改，符合 slog 契约
		r = r.Clone()
		r.AddAttrs(attrs...)
	}

	return h.base.Handle(ctx, r)
}

// WithAttrs 返回带额外属性的新 handler
func (h *EnrichHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &EnrichHandler{
		base: h.base.WithAttrs(attrs),
	}
}

// WithGroup 返回带分组的新 handler
func (h *EnrichHandler) WithGroup(name string) slog.Handler {
	return &EnrichHandler{
		base: h.base.WithGroup(name),
	}
}
