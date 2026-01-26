package xctx_test

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// Example_quickStart 演示 xctx 包的典型使用场景。
//
// 在请求入口（如 HTTP 中间件）中注入身份和追踪信息，
// 然后在业务代码中读取这些信息用于日志记录或业务决策。
func Example_quickStart() {
	ctx := context.Background()

	// 1. 在请求入口注入身份信息
	ctx, _ = xctx.WithPlatformID(ctx, "platform-001")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-002")
	ctx, _ = xctx.WithTenantName(ctx, "测试租户")

	// 2. 自动生成追踪信息（若上游未传递）
	ctx, _ = xctx.EnsureTrace(ctx)

	// 3. 在业务代码中读取
	fmt.Printf("PlatformID: %s\n", xctx.PlatformID(ctx))
	fmt.Printf("TenantID: %s\n", xctx.TenantID(ctx))
	fmt.Printf("TraceID 已生成: %v\n", xctx.TraceID(ctx) != "")

	// Output:
	// PlatformID: platform-001
	// TenantID: tenant-002
	// TraceID 已生成: true
}

// Example_requireFunctions 演示 Require 系列函数的错误处理。
//
// Require 系列函数在值缺失时返回错误，适用于必须有身份信息的业务场景。
func Example_requireFunctions() {
	ctx := context.Background()

	// 未设置租户 ID 时返回错误
	_, err := xctx.RequireTenantID(ctx)
	fmt.Printf("未设置时: %v\n", err == xctx.ErrMissingTenantID)

	// 设置后可正常获取
	ctx, _ = xctx.WithTenantID(ctx, "tenant-123")
	tenantID, err := xctx.RequireTenantID(ctx)
	fmt.Printf("设置后: %s, err=nil: %v\n", tenantID, err == nil)

	// Output:
	// 未设置时: true
	// 设置后: tenant-123, err=nil: true
}

// Example_deploymentTypeValidation 演示部署类型的验证逻辑。
//
// GetDeploymentType 会验证部署类型有效性，而 DeploymentTypeRaw 只返回原始值。
func Example_deploymentTypeValidation() {
	ctx := context.Background()

	// 注入有效的部署类型
	ctx, err := xctx.WithDeploymentType(ctx, xctx.DeploymentLocal)
	if err != nil {
		fmt.Println("注入失败:", err)
		return
	}

	// GetDeploymentType 返回验证后的值
	dt, err := xctx.GetDeploymentType(ctx)
	if err != nil {
		fmt.Println("获取失败:", err)
		return
	}
	fmt.Printf("类型: %s, IsLocal: %v\n", dt, dt.IsLocal())

	// DeploymentTypeRaw 直接返回原始值（无验证）
	raw := xctx.DeploymentTypeRaw(ctx)
	fmt.Printf("原始值: %s\n", raw)

	// Output:
	// 类型: LOCAL, IsLocal: true
	// 原始值: LOCAL
}

// Example_tracePreservation 演示追踪信息的传播与保留。
//
// EnsureTrace 遵循"有则沿用，无则生成"的语义，不会覆盖已存在的追踪信息。
func Example_tracePreservation() {
	ctx := context.Background()

	// 模拟上游传递的追踪信息
	upstreamTrace := xctx.Trace{
		TraceID:   "0af7651916cd43dd8448eb211c80319c",
		SpanID:    "b7ad6b7169203331",
		RequestID: "req-from-upstream",
	}
	ctx, _ = xctx.WithTrace(ctx, upstreamTrace)

	// EnsureTrace 保留已存在的值
	ctx, _ = xctx.EnsureTrace(ctx)

	// 验证原有值未被覆盖
	trace := xctx.GetTrace(ctx)
	fmt.Printf("TraceID 保留: %v\n", trace.TraceID == upstreamTrace.TraceID)
	fmt.Printf("SpanID 保留: %v\n", trace.SpanID == upstreamTrace.SpanID)
	fmt.Printf("RequestID 保留: %v\n", trace.RequestID == upstreamTrace.RequestID)

	// Output:
	// TraceID 保留: true
	// SpanID 保留: true
	// RequestID 保留: true
}
