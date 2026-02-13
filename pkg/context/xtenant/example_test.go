package xtenant_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// Example_quickStart 演示 xtenant 包的典型使用场景。
//
// 从 HTTP 请求提取租户信息并注入 context，然后在业务代码中使用。
func Example_quickStart() {
	// 模拟收到的 HTTP 请求
	req, err := http.NewRequest("GET", "/api/data", nil)
	if err != nil {
		fmt.Println("创建请求失败:", err)
		return
	}
	req.Header.Set("X-Tenant-ID", "tenant-123")
	req.Header.Set("X-Tenant-Name", "测试租户")

	// 从 HTTP Header 提取租户信息
	info := xtenant.ExtractFromHTTPRequest(req)
	fmt.Printf("TenantID: %s\n", info.TenantID)
	fmt.Printf("TenantName: %s\n", info.TenantName)

	// 注入到 context
	ctx := context.Background()
	ctx, err = xtenant.WithTenantInfo(ctx, info)
	if err != nil {
		fmt.Println("注入失败:", err)
		return
	}

	// 业务代码中读取
	fmt.Printf("从 context 读取: %s\n", xtenant.TenantID(ctx))

	// Output:
	// TenantID: tenant-123
	// TenantName: 测试租户
	// 从 context 读取: tenant-123
}

// ExampleTenantInfo_Validate 演示租户信息验证。
func ExampleTenantInfo_Validate() {
	// 完整信息通过验证
	info := xtenant.TenantInfo{
		TenantID:   "tenant-123",
		TenantName: "测试租户",
	}
	fmt.Printf("完整信息: %v\n", info.Validate() == nil)

	// 缺少 TenantID 返回错误
	info.TenantID = ""
	fmt.Printf("缺少 ID: %v\n", info.Validate() == xtenant.ErrEmptyTenantID)

	// Output:
	// 完整信息: true
	// 缺少 ID: true
}

// ExampleHTTPMiddleware 演示 HTTP 中间件的使用。
func ExampleHTTPMiddleware() {
	// 创建带选项的中间件
	middleware := xtenant.HTTPMiddlewareWithOptions(
		xtenant.WithRequireTenantID(), // 要求 TenantID 必须存在
		xtenant.WithEnsureTrace(),     // 自动生成追踪信息
	)

	// 使用中间件包装 handler
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		fmt.Printf("TenantID: %s\n", xtenant.TenantID(ctx))
	}))

	// 模拟请求
	req, err := http.NewRequest("GET", "/api/data", nil)
	if err != nil {
		fmt.Println("创建请求失败:", err)
		return
	}
	req.Header.Set("X-Tenant-ID", "tenant-123")

	// 执行 handler
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Output:
	// TenantID: tenant-123
}

// ExampleInjectToRequest 演示跨服务调用时的信息传播。
func ExampleInjectToRequest() {
	// 准备带租户信息的 context
	ctx := context.Background()
	var err error
	ctx, err = xtenant.WithTenantID(ctx, "tenant-123")
	if err != nil {
		fmt.Println("注入 TenantID 失败:", err)
		return
	}
	ctx, err = xtenant.WithTenantName(ctx, "测试租户")
	if err != nil {
		fmt.Println("注入 TenantName 失败:", err)
		return
	}

	// 创建下游请求
	downstreamReq, err := http.NewRequest("GET", "http://downstream/api", nil)
	if err != nil {
		fmt.Println("创建请求失败:", err)
		return
	}

	// 将 context 中的信息注入到请求 Header
	xtenant.InjectToRequest(ctx, downstreamReq)

	// 验证 Header 已设置
	fmt.Printf("X-Tenant-ID: %s\n", downstreamReq.Header.Get("X-Tenant-ID"))
	fmt.Printf("X-Tenant-Name: %s\n", downstreamReq.Header.Get("X-Tenant-Name"))

	// Output:
	// X-Tenant-ID: tenant-123
	// X-Tenant-Name: 测试租户
}

// ExampleRequireTenantID 演示强制获取租户 ID。
func ExampleRequireTenantID() {
	ctx := context.Background()

	// 未设置时返回错误
	_, err := xtenant.RequireTenantID(ctx)
	fmt.Printf("未设置: %v\n", err != nil)

	// 设置后可正常获取
	ctx, err = xtenant.WithTenantID(ctx, "tenant-123")
	if err != nil {
		fmt.Println("注入失败:", err)
		return
	}
	id, err := xtenant.RequireTenantID(ctx)
	fmt.Printf("设置后: %s, err=nil: %v\n", id, err == nil)

	// Output:
	// 未设置: true
	// 设置后: tenant-123, err=nil: true
}
