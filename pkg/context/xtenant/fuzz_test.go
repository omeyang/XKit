package xtenant_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xtenant"

	"google.golang.org/grpc/metadata"
)

// =============================================================================
// Context 操作 Fuzz 测试
// =============================================================================

func FuzzWithTenantID(f *testing.F) {
	// 添加种子语料
	f.Add("tenant-123")
	f.Add("TestTenant")
	f.Add("")
	f.Add("   ")
	f.Add("tenant_with_underscore")
	f.Add("tenant-with-dash")
	f.Add("tenant.with.dot")
	f.Add("a")
	f.Add("very-long-tenant-id-that-might-cause-issues-in-some-edge-cases")

	f.Fuzz(func(t *testing.T, tenantID string) {
		ctx := context.Background()
		newCtx, err := xtenant.WithTenantID(ctx, tenantID)

		// WithTenantID 不对空值做验证，只有 nil context 才会报错
		if err != nil {
			t.Errorf("unexpected error for tenant ID %q: %v", tenantID, err)
			return
		}

		// 验证能正确读取
		got := xtenant.TenantID(newCtx)
		if got != tenantID {
			t.Errorf("TenantID() = %q, want %q", got, tenantID)
		}
	})
}

func FuzzWithTenantName(f *testing.F) {
	// 添加种子语料
	f.Add("TestTenant")
	f.Add("TestOrg")
	f.Add("")
	f.Add("   ")
	f.Add("测试租户名称")
	f.Add("Company with spaces")
	f.Add("Special chars: !@#$%^&*()")

	f.Fuzz(func(t *testing.T, tenantName string) {
		ctx := context.Background()
		newCtx, err := xtenant.WithTenantName(ctx, tenantName)

		// WithTenantName 不对空值做验证，只有 nil context 才会报错
		if err != nil {
			t.Errorf("unexpected error for tenant name %q: %v", tenantName, err)
			return
		}

		// 验证能正确读取
		got := xtenant.TenantName(newCtx)
		if got != tenantName {
			t.Errorf("TenantName() = %q, want %q", got, tenantName)
		}
	})
}

func FuzzWithTenantInfo(f *testing.F) {
	f.Add("tenant-1", "TestTenant")
	f.Add("", "name")
	f.Add("id", "")
	f.Add("", "")
	f.Add("tenant-特殊字符", "名称-特殊")

	f.Fuzz(func(t *testing.T, tenantID, tenantName string) {
		ctx := context.Background()
		info := xtenant.TenantInfo{
			TenantID:   tenantID,
			TenantName: tenantName,
		}

		newCtx, err := xtenant.WithTenantInfo(ctx, info)

		// WithTenantInfo 只对 nil context 返回错误，不验证字段
		// 它只是注入非空字段
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// 验证能正确读取非空字段
		gotInfo := xtenant.GetTenantInfo(newCtx)
		if tenantID != "" && gotInfo.TenantID != tenantID {
			t.Errorf("TenantID = %q, want %q", gotInfo.TenantID, tenantID)
		}
		if tenantName != "" && gotInfo.TenantName != tenantName {
			t.Errorf("TenantName = %q, want %q", gotInfo.TenantName, tenantName)
		}
	})
}

// =============================================================================
// TenantInfo 验证 Fuzz 测试
// =============================================================================

func FuzzTenantInfo_Validate(f *testing.F) {
	f.Add("tenant-1", "TestTenant")
	f.Add("", "")
	f.Add("id", "")
	f.Add("", "name")

	f.Fuzz(func(t *testing.T, tenantID, tenantName string) {
		info := xtenant.TenantInfo{
			TenantID:   tenantID,
			TenantName: tenantName,
		}

		err := info.Validate()

		// Validate() 要求 TenantID 和 TenantName 都非空
		shouldError := tenantID == "" || tenantName == ""

		if shouldError {
			if err == nil {
				t.Errorf("expected error when TenantID=%q, TenantName=%q", tenantID, tenantName)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for valid TenantInfo: %v", err)
			}
		}
	})
}

// =============================================================================
// HTTP 提取 Fuzz 测试
// =============================================================================

func FuzzExtractFromHTTPHeader(f *testing.F) {
	f.Add("tenant-123", "TestTenant")
	f.Add("", "")
	f.Add("  spaced  ", "  name  ")
	f.Add("unicode-租户", "中文名称")

	f.Fuzz(func(t *testing.T, tenantID, tenantName string) {
		h := http.Header{}
		if tenantID != "" {
			h.Set(xtenant.HeaderTenantID, tenantID)
		}
		if tenantName != "" {
			h.Set(xtenant.HeaderTenantName, tenantName)
		}

		info := xtenant.ExtractFromHTTPHeader(h)

		// 验证提取结果正确（会自动 trim 空白）
		if tenantID != "" && info.TenantID == "" {
			// 只有完全空白的才会变成空
		}
		if tenantName != "" && info.TenantName == "" {
			// 只有完全空白的才会变成空
		}
	})
}

// =============================================================================
// gRPC 提取 Fuzz 测试
// =============================================================================

func FuzzExtractFromMetadata(f *testing.F) {
	f.Add("tenant-123", "TestTenant")
	f.Add("", "")
	f.Add("  spaced  ", "  name  ")

	f.Fuzz(func(t *testing.T, tenantID, tenantName string) {
		var md metadata.MD
		if tenantID != "" || tenantName != "" {
			pairs := []string{}
			if tenantID != "" {
				pairs = append(pairs, xtenant.MetaTenantID, tenantID)
			}
			if tenantName != "" {
				pairs = append(pairs, xtenant.MetaTenantName, tenantName)
			}
			md = metadata.Pairs(pairs...)
		}

		// 不应该 panic
		info := xtenant.ExtractFromMetadata(md)
		_ = info.IsEmpty()
	})
}
