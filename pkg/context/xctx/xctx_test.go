package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// Context 隔离测试
// =============================================================================

func TestContextIsolation(t *testing.T) {
	t.Parallel()

	parent, err := xctx.WithPlatformID(context.Background(), "parent-platform")
	if err != nil {
		t.Fatalf("WithPlatformID() error = %v", err)
	}
	child, err := xctx.WithPlatformID(parent, "child-platform")
	if err != nil {
		t.Fatalf("WithPlatformID() error = %v", err)
	}

	if got := xctx.PlatformID(parent); got != "parent-platform" {
		t.Errorf("parent PlatformID() = %q, want %q", got, "parent-platform")
	}
	if got := xctx.PlatformID(child); got != "child-platform" {
		t.Errorf("child PlatformID() = %q, want %q", got, "child-platform")
	}
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx, _ := xctx.WithPlatformID(context.Background(), "platform")
	ctx, _ = xctx.WithTenantID(ctx, "tenant")
	ctx, _ = xctx.WithTenantName(ctx, "name")
	ctx, _ = xctx.WithTraceID(ctx, "trace")
	ctx, _ = xctx.WithSpanID(ctx, "span")
	ctx, _ = xctx.WithRequestID(ctx, "req")

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			if got := xctx.PlatformID(ctx); got != "platform" {
				t.Errorf("PlatformID() = %q, want %q", got, "platform")
			}
			if got := xctx.TraceID(ctx); got != "trace" {
				t.Errorf("TraceID() = %q, want %q", got, "trace")
			}
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// =============================================================================
// 错误类型测试
// =============================================================================

func TestErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrMissingPlatformID", xctx.ErrMissingPlatformID, "xctx: missing platform_id"},
		{"ErrMissingTenantID", xctx.ErrMissingTenantID, "xctx: missing tenant_id"},
		{"ErrMissingTenantName", xctx.ErrMissingTenantName, "xctx: missing tenant_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); got != tt.want {
				t.Errorf("%s.Error() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	t.Parallel()

	// 测试错误可以被正确包装和解包
	ctx := context.Background()
	_, err := xctx.RequireTenantID(ctx)
	wrapped := fmt.Errorf("business error: %w", err)

	if !errors.Is(wrapped, xctx.ErrMissingTenantID) {
		t.Error("wrapped error should be unwrappable to ErrMissingTenantID")
	}
}

// =============================================================================
// Ensure + GetIdentity 组合场景测试
// =============================================================================

func TestEnsureAndGetIdentity_RealWorldScenario(t *testing.T) {
	t.Parallel()

	// 模拟 HTTP 中间件场景：
	// 1. 入口处 EnsureTrace 确保追踪信息
	// 2. 业务层 GetIdentity().Validate() 检查身份信息

	t.Run("入口中间件场景", func(t *testing.T) {
		t.Parallel()

		// 模拟请求入口：确保追踪信息
		ctx, err := xctx.EnsureTrace(context.Background())
		if err != nil {
			t.Fatalf("EnsureTrace() error = %v", err)
		}

		// 验证追踪信息已就绪
		if xctx.TraceID(ctx) == "" {
			t.Error("TraceID should be available after EnsureTrace")
		}

		// 模拟业务层：身份信息缺失应报错
		id := xctx.GetIdentity(ctx)
		if err := id.Validate(); err == nil {
			t.Error("Validate() should fail when identity not set")
		}
	})

	t.Run("完整请求链路场景", func(t *testing.T) {
		t.Parallel()

		// 1. 入口确保追踪
		ctx, err := xctx.EnsureTrace(context.Background())
		if err != nil {
			t.Fatalf("EnsureTrace() error = %v", err)
		}

		// 2. 认证中间件注入身份（假设从 JWT 解析）
		ctx, _ = xctx.WithPlatformID(ctx, "platform-saas")
		ctx, _ = xctx.WithTenantID(ctx, "tenant-001")
		ctx, _ = xctx.WithTenantName(ctx, "TestCompany")

		// 3. 业务层获取所有信息
		id := xctx.GetIdentity(ctx)
		if err := id.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		if id.PlatformID != "platform-saas" {
			t.Errorf("PlatformID = %q, want %q", id.PlatformID, "platform-saas")
		}
		if id.TenantID != "tenant-001" {
			t.Errorf("TenantID = %q, want %q", id.TenantID, "tenant-001")
		}
		if id.TenantName != "TestCompany" {
			t.Errorf("TenantName = %q, want %q", id.TenantName, "TestCompany")
		}

		// 追踪信息也应该可用
		if xctx.TraceID(ctx) == "" {
			t.Error("TraceID should still be available")
		}
	})
}
