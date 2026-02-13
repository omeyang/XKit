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

	// 设计决策: 直接调用 AppendIdentityAttrs 而非先做快速路径检查。
	// 前版本先逐字段检查是否全空再调用 Append，导致非空场景（生产常见路径）
	// 每个字段被 context.Value 查找两次。去除冗余检查后，非空路径减少 N 次查找。
	attrs := AppendIdentityAttrs(make([]slog.Attr, 0, identityFieldCount), ctx)
	if len(attrs) == 0 {
		return nil
	}
	return attrs
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

	attrs := AppendTraceAttrs(make([]slog.Attr, 0, traceFieldCount), ctx)
	if len(attrs) == 0 {
		return nil
	}
	return attrs
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

	attrs := AppendPlatformAttrs(make([]slog.Attr, 0, platformFieldCount), ctx)
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

// =============================================================================
// 合并 slog 集成
// =============================================================================

// LogAttrs 从 context 提取所有上下文信息，转换为 slog.Attr 切片
// 包含身份信息、追踪信息、部署类型和平台信息，只返回非空/已设置的字段。
//
// deployment_type 为必填字段，缺失或无效时返回错误。
// 错误时仍返回已收集的部分属性（identity/trace/platform），调用方可自行决定是否使用。
// 如果 context 中完全没有任何字段，返回 (nil, err) 避免不必要的分配。
func LogAttrs(ctx context.Context) ([]slog.Attr, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	// 先检查必填的 deployment_type，在错误路径上延迟分配
	dt, err := GetDeploymentType(ctx)
	if err != nil {
		// 部署类型缺失/无效，尝试收集其他字段作为部分结果
		var partial []slog.Attr
		partial = AppendIdentityAttrs(partial, ctx)
		partial = AppendTraceAttrs(partial, ctx)
		partial = AppendPlatformAttrs(partial, ctx)
		return partial, err
	}

	// 成功路径：预分配完整容量
	attrs := make([]slog.Attr, 0, identityFieldCount+traceFieldCount+deploymentFieldCount+platformFieldCount)
	attrs = AppendIdentityAttrs(attrs, ctx)
	attrs = AppendTraceAttrs(attrs, ctx)
	attrs = AppendPlatformAttrs(attrs, ctx)
	attrs = append(attrs, slog.String(KeyDeploymentType, dt.String()))

	return attrs, nil
}
