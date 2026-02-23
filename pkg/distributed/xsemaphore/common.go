package xsemaphore

import (
	"context"
	"time"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// applyDefaultTimeout 如果 context 没有 deadline 且配置了默认超时，则应用默认超时
// 返回新的 context 和 cancel 函数（如果创建了新 context）
//
// 当 ctx 为 nil 时直接返回，不做超时包装——后续 validateCommonParams 会返回 ErrNilContext。
func applyDefaultTimeout(ctx context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	if defaultTimeout <= 0 || ctx == nil {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		// 已有 deadline，不覆盖
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// =============================================================================
// 公共辅助函数
// =============================================================================

// waitForRetry 等待重试间隔
// 返回 nil 表示等待完成，返回 error 表示 context 被取消
func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// applyAcquireOptions 应用获取选项并返回配置
func applyAcquireOptions(opts []AcquireOption) *acquireOptions {
	cfg := defaultAcquireOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// resolveTenantID 解析租户 ID
// 优先使用显式设置的 tenantID，否则从 context 中提取
func resolveTenantID(ctx context.Context, explicitID string) string {
	if explicitID != "" {
		return explicitID
	}
	return xtenant.TenantID(ctx)
}

// validateCommonParams 校验公共参数：context、closed 状态和资源名
func validateCommonParams(ctx context.Context, resource string, closed bool) error {
	if ctx == nil {
		return ErrNilContext
	}
	if closed {
		return ErrSemaphoreClosed
	}
	return validateResource(resource)
}

// prepareAcquireCommon 准备获取许可的公共逻辑
// 返回：配置、租户ID、错误
func prepareAcquireCommon(ctx context.Context, resource string, opts []AcquireOption, closed bool) (*acquireOptions, string, error) {
	if err := validateCommonParams(ctx, resource, closed); err != nil {
		return nil, "", err
	}

	cfg := applyAcquireOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, "", err
	}

	tenantID := resolveTenantID(ctx, cfg.tenantID)
	if err := validateTenantID(tenantID); err != nil {
		return nil, "", err
	}
	return cfg, tenantID, nil
}

// applyQueryOptions 应用查询选项并返回配置
func applyQueryOptions(opts []QueryOption) *queryOptions {
	cfg := defaultQueryOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// prepareQueryCommon 准备查询的公共逻辑
func prepareQueryCommon(ctx context.Context, resource string, opts []QueryOption, closed bool) (*queryOptions, string, error) {
	if err := validateCommonParams(ctx, resource, closed); err != nil {
		return nil, "", err
	}

	cfg := applyQueryOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, "", err
	}
	tenantID := resolveTenantID(ctx, cfg.tenantID)
	if err := validateTenantID(tenantID); err != nil {
		return nil, "", err
	}
	return cfg, tenantID, nil
}
