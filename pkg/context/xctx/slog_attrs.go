package xctx

import (
	"context"
	"log/slog"
)

// =============================================================================
// Identity slog 集成
// =============================================================================

// AppendIdentityAttrs 将 context 中的身份信息追加到现有切片。
// 零分配热路径优化：传入预分配的切片，只追加非空的身份信息字段。
func AppendIdentityAttrs(attrs []slog.Attr, ctx context.Context) []slog.Attr {
	if ctx == nil {
		return attrs
	}

	if v := PlatformID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyPlatformID, v))
	}
	if v := TenantID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyTenantID, v))
	}
	if v := TenantName(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyTenantName, v))
	}

	return attrs
}

// IdentityAttrs 从 context 提取身份信息，转换为 slog.Attr 切片
//
// 只返回非空的身份信息，如果都为空则返回 nil。
// 注意：每次调用会分配新切片。热路径建议使用 AppendIdentityAttrs。
func IdentityAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	// 快速检查：如果所有字段都为空，直接返回 nil 避免分配
	if PlatformID(ctx) == "" && TenantID(ctx) == "" && TenantName(ctx) == "" {
		return nil
	}

	return AppendIdentityAttrs(make([]slog.Attr, 0, IdentityFieldCount), ctx)
}

// =============================================================================
// Trace slog 集成
// =============================================================================

// AppendTraceAttrs 将 context 中的追踪信息追加到现有切片。
// 零分配热路径优化：传入预分配的切片，只追加非空的追踪信息字段。
func AppendTraceAttrs(attrs []slog.Attr, ctx context.Context) []slog.Attr {
	if ctx == nil {
		return attrs
	}

	if v := TraceID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyTraceID, v))
	}
	if v := SpanID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeySpanID, v))
	}
	if v := RequestID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyRequestID, v))
	}
	if v := TraceFlags(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyTraceFlags, v))
	}

	return attrs
}

// TraceAttrs 从 context 提取追踪信息，转换为 slog.Attr 切片
//
// 只返回非空的追踪信息，如果都为空则返回 nil。
// 注意：每次调用会分配新切片。热路径建议使用 AppendTraceAttrs。
func TraceAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	// 快速检查：如果所有字段都为空，直接返回 nil 避免分配
	if TraceID(ctx) == "" && SpanID(ctx) == "" && RequestID(ctx) == "" && TraceFlags(ctx) == "" {
		return nil
	}

	return AppendTraceAttrs(make([]slog.Attr, 0, TraceFieldCount), ctx)
}

// =============================================================================
// Deployment slog 集成
// =============================================================================

// DeploymentAttrs 从 context 提取部署类型，转换为 slog.Attr 切片
//
// 如果 context 中缺少或包含非法 deployment_type，返回错误。
func DeploymentAttrs(ctx context.Context) ([]slog.Attr, error) {
	dt, err := GetDeploymentType(ctx)
	if err != nil {
		return nil, err
	}
	return []slog.Attr{slog.String(KeyDeploymentType, dt.String())}, nil
}

// =============================================================================
// Platform slog 集成
// =============================================================================

// AppendPlatformAttrs 将 context 中的平台信息追加到现有切片。
// 零分配热路径优化：传入预分配的切片，只追加已设置的平台信息字段。
func AppendPlatformAttrs(attrs []slog.Attr, ctx context.Context) []slog.Attr {
	if ctx == nil {
		return attrs
	}

	if v, ok := HasParent(ctx); ok {
		attrs = append(attrs, slog.Bool(KeyHasParent, v))
	}
	if v := UnclassRegionID(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyUnclassRegionID, v))
	}

	return attrs
}

// PlatformAttrs 从 context 提取平台信息，转换为 slog.Attr 切片
//
// 只返回已设置的平台信息，如果都未设置则返回 nil。
// 注意：每次调用会分配新切片。热路径建议使用 AppendPlatformAttrs。
func PlatformAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	// 快速检查：如果所有字段都未设置，直接返回 nil 避免分配
	_, hasParentOK := HasParent(ctx)
	if !hasParentOK && UnclassRegionID(ctx) == "" {
		return nil
	}

	return AppendPlatformAttrs(make([]slog.Attr, 0, PlatformFieldCount), ctx)
}

// =============================================================================
// 合并 slog 集成
// =============================================================================

// LogAttrs 从 context 提取所有上下文信息，转换为 slog.Attr 切片
// 包含身份信息、追踪信息、部署类型和平台信息，只返回非空/已设置的字段。
//
// 注意：deployment_type 为必填字段，缺失时会返回错误；其他字段仍会尽可能返回已存在值。
func LogAttrs(ctx context.Context) ([]slog.Attr, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	// 使用 Append 变体直接追加到预分配切片，避免中间分配
	attrs := make([]slog.Attr, 0, IdentityFieldCount+TraceFieldCount+DeploymentFieldCount+PlatformFieldCount)
	attrs = AppendIdentityAttrs(attrs, ctx)
	attrs = AppendTraceAttrs(attrs, ctx)
	attrs = AppendPlatformAttrs(attrs, ctx)

	dt, err := GetDeploymentType(ctx)
	if err != nil {
		return attrs, err
	}
	attrs = append(attrs, slog.String(KeyDeploymentType, dt.String()))

	return attrs, nil
}
