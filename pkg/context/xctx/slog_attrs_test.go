package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// slog 集成测试
// =============================================================================

// testAttrCase 定义属性测试用例
type testAttrCase struct {
	name      string
	values    []string // 三个字段的值
	wantCount int
}

// runAttrTests 通用属性测试执行器
func runAttrTests(
	t *testing.T,
	attrName string,
	cases []testAttrCase,
	setters []func(context.Context, string) (context.Context, error),
	getAttrs func(context.Context) []any,
) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			for i, setter := range setters {
				if tt.values[i] != "" {
					var err error
					ctx, err = setter(ctx, tt.values[i])
					require.NoErrorf(t, err, "setter[%d]()", i)
				}
			}
			attrs := getAttrs(ctx)
			assert.Lenf(t, attrs, tt.wantCount, "%s() len", attrName)
		})
	}
}

// assertSingleAttr 断言属性切片恰好包含一个元素，且 key 和 value 匹配
func assertSingleAttr(t *testing.T, attrs []slog.Attr, wantKey, wantValue string) {
	t.Helper()
	require.Len(t, attrs, 1, "PlatformAttrs() should return exactly 1 attr")
	assert.Equal(t, wantKey, attrs[0].Key, "attr key")
	assert.Equal(t, wantValue, attrs[0].Value.String(), "attr value")
}

func TestIdentityAttrs(t *testing.T) {
	t.Parallel()

	cases := []testAttrCase{
		{"全部为空", []string{"", "", ""}, 0},
		{"只有平台", []string{"p1", "", ""}, 1},
		{"只有租户ID", []string{"", "t1", ""}, 1},
		{"全部存在", []string{"p1", "t1", "n1"}, 3},
	}
	setters := []func(context.Context, string) (context.Context, error){
		xctx.WithPlatformID,
		xctx.WithTenantID,
		xctx.WithTenantName,
	}
	// 包装函数以匹配通用签名
	getAttrs := func(ctx context.Context) []any {
		attrs := xctx.IdentityAttrs(ctx)
		result := make([]any, len(attrs))
		for i, a := range attrs {
			result[i] = a
		}
		return result
	}
	runAttrTests(t, "IdentityAttrs", cases, setters, getAttrs)
}

func TestAppendIdentityAttrs_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	attrs := make([]slog.Attr, 0, 3)
	result := xctx.AppendIdentityAttrs(attrs, nilCtx)
	assert.Empty(t, result, "AppendIdentityAttrs(nil) should return unchanged slice")
}

func TestAppendTraceAttrs_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	attrs := make([]slog.Attr, 0, 4)
	result := xctx.AppendTraceAttrs(attrs, nilCtx)
	assert.Empty(t, result, "AppendTraceAttrs(nil) should return unchanged slice")
}

func TestAppendPlatformAttrs_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	attrs := make([]slog.Attr, 0, 2)
	result := xctx.AppendPlatformAttrs(attrs, nilCtx)
	assert.Empty(t, result, "AppendPlatformAttrs(nil) should return unchanged slice")
}

func TestTraceAttrs(t *testing.T) {
	t.Parallel()

	cases := []testAttrCase{
		{"全部为空", []string{"", "", ""}, 0},
		{"只有 trace", []string{"t1", "", ""}, 1},
		{"全部存在", []string{"t1", "s1", "r1"}, 3},
	}
	setters := []func(context.Context, string) (context.Context, error){
		xctx.WithTraceID,
		xctx.WithSpanID,
		xctx.WithRequestID,
	}
	// 包装函数以匹配通用签名
	getAttrs := func(ctx context.Context) []any {
		attrs := xctx.TraceAttrs(ctx)
		result := make([]any, len(attrs))
		for i, a := range attrs {
			result[i] = a
		}
		return result
	}
	runAttrTests(t, "TraceAttrs", cases, setters, getAttrs)

	t.Run("包含TraceFlags", func(t *testing.T) {
		t.Parallel()

		ctx, _ := xctx.WithTraceID(context.Background(), "t1")
		ctx, _ = xctx.WithTraceFlags(ctx, "01")
		attrs := xctx.TraceAttrs(ctx)
		assert.Len(t, attrs, 2, "TraceAttrs() with TraceFlags len")
		// 验证 TraceFlags 属性存在
		found := false
		for _, a := range attrs {
			if a.Key == xctx.KeyTraceFlags {
				assert.Equal(t, "01", a.Value.String(), "TraceFlags value")
				found = true
			}
		}
		assert.True(t, found, "TraceFlags attr should be present")
	})
}

func TestDeploymentAttrs(t *testing.T) {
	t.Parallel()

	t.Run("有效部署类型", func(t *testing.T) {
		t.Parallel()

		ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentSaaS)
		if err != nil {
			t.Fatalf("WithDeploymentType() error = %v", err)
		}
		attrs, err := xctx.DeploymentAttrs(ctx)
		if err != nil {
			t.Fatalf("DeploymentAttrs() error = %v", err)
		}
		if len(attrs) != 1 {
			t.Errorf("DeploymentAttrs() len = %d, want 1", len(attrs))
		}
		if attrs[0].Key != xctx.KeyDeploymentType {
			t.Errorf("attr key = %q, want %q", attrs[0].Key, xctx.KeyDeploymentType)
		}
		if attrs[0].Value.String() != "SAAS" {
			t.Errorf("attr value = %q, want %q", attrs[0].Value.String(), "SAAS")
		}
	})

	t.Run("空部署类型返回错误", func(t *testing.T) {
		t.Parallel()

		_, err := xctx.DeploymentAttrs(context.Background())
		if !errors.Is(err, xctx.ErrMissingDeploymentType) {
			t.Errorf("DeploymentAttrs(empty) error = %v, want %v", err, xctx.ErrMissingDeploymentType)
		}
	})

	t.Run("nil context返回错误", func(t *testing.T) {
		t.Parallel()

		var nilCtx context.Context
		_, err := xctx.DeploymentAttrs(nilCtx)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("DeploymentAttrs(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func TestPlatformAttrs(t *testing.T) {
	t.Parallel()

	t.Run("空context返回nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, xctx.PlatformAttrs(context.Background()), "PlatformAttrs(empty)")
	})

	t.Run("HasParent=true", func(t *testing.T) {
		t.Parallel()

		ctx, err := xctx.WithHasParent(context.Background(), true)
		require.NoError(t, err, "WithHasParent()")
		assertSingleAttr(t, xctx.PlatformAttrs(ctx), xctx.KeyHasParent, "true")
	})

	t.Run("HasParent=false仍输出字段", func(t *testing.T) {
		t.Parallel()

		ctx, err := xctx.WithHasParent(context.Background(), false)
		require.NoError(t, err, "WithHasParent()")
		assertSingleAttr(t, xctx.PlatformAttrs(ctx), xctx.KeyHasParent, "false")
	})

	t.Run("UnclassRegionID", func(t *testing.T) {
		t.Parallel()

		ctx, err := xctx.WithUnclassRegionID(context.Background(), "region-001")
		require.NoError(t, err, "WithUnclassRegionID()")
		assertSingleAttr(t, xctx.PlatformAttrs(ctx), xctx.KeyUnclassRegionID, "region-001")
	})

	t.Run("组合字段", func(t *testing.T) {
		t.Parallel()

		ctx, err := xctx.WithHasParent(context.Background(), true)
		require.NoError(t, err, "WithHasParent()")
		ctx, err = xctx.WithUnclassRegionID(ctx, "region-002")
		require.NoError(t, err, "WithUnclassRegionID()")
		attrs := xctx.PlatformAttrs(ctx)
		require.Len(t, attrs, 2, "PlatformAttrs() len")
		assert.Equal(t, xctx.KeyHasParent, attrs[0].Key, "attr[0].key")
		assert.Equal(t, xctx.KeyUnclassRegionID, attrs[1].Key, "attr[1].key")
	})
}

func TestLogAttrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		identity   bool
		trace      bool
		deployment bool
		platform   bool
		wantCount  int
		wantErr    error
	}{
		{"全部为空", false, false, false, false, 0, xctx.ErrMissingDeploymentType},
		{"只有 identity", true, false, false, false, 3, xctx.ErrMissingDeploymentType},
		{"只有 trace", false, true, false, false, 3, xctx.ErrMissingDeploymentType},
		{"只有 platform", false, false, false, true, 2, xctx.ErrMissingDeploymentType},
		{"只有 deployment", false, false, true, false, 1, nil},
		{"deployment + platform", false, false, true, true, 3, nil},
		{"全部存在", true, true, true, true, 9, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := buildLogAttrsContext(t, tt.identity, tt.trace, tt.deployment, tt.platform)
			attrs, err := xctx.LogAttrs(ctx)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "LogAttrs() error")
			} else {
				require.NoError(t, err, "LogAttrs()")
			}
			assert.Len(t, attrs, tt.wantCount, "LogAttrs() len")
		})
	}
}

// buildLogAttrsContext 根据标志位构建包含不同字段组合的 context
func buildLogAttrsContext(t *testing.T, identity, trace, deployment, platform bool) context.Context {
	t.Helper()
	ctx := context.Background()
	if identity {
		ctx, _ = xctx.WithPlatformID(ctx, "p1")
		ctx, _ = xctx.WithTenantID(ctx, "t1")
		ctx, _ = xctx.WithTenantName(ctx, "n1")
	}
	if trace {
		ctx, _ = xctx.WithTraceID(ctx, "trace1")
		ctx, _ = xctx.WithSpanID(ctx, "span1")
		ctx, _ = xctx.WithRequestID(ctx, "req1")
	}
	if platform {
		ctx, _ = xctx.WithHasParent(ctx, true)
		ctx, _ = xctx.WithUnclassRegionID(ctx, "region-1")
	}
	if deployment {
		var err error
		ctx, err = xctx.WithDeploymentType(ctx, xctx.DeploymentSaaS)
		require.NoError(t, err, "WithDeploymentType()")
	}
	return ctx
}

func TestLogAttrs_Values(t *testing.T) {
	t.Parallel()

	ctx, _ := xctx.WithPlatformID(context.Background(), "platform-abc")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-123")
	ctx, _ = xctx.WithTenantName(ctx, "TestTenant")
	ctx, _ = xctx.WithTraceID(ctx, "trace-xyz")
	ctx, _ = xctx.WithSpanID(ctx, "span-456")
	ctx, _ = xctx.WithRequestID(ctx, "req-789")
	ctx, _ = xctx.WithHasParent(ctx, true)
	ctx, _ = xctx.WithUnclassRegionID(ctx, "region-001")
	var err error
	ctx, err = xctx.WithDeploymentType(ctx, xctx.DeploymentLocal)
	if err != nil {
		t.Fatalf("WithDeploymentType() error = %v", err)
	}

	attrs, err := xctx.LogAttrs(ctx)
	if err != nil {
		t.Fatalf("LogAttrs() error = %v", err)
	}

	attrMap := make(map[string]string)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr.Value.String()
	}

	expected := map[string]string{
		xctx.KeyPlatformID:      "platform-abc",
		xctx.KeyTenantID:        "tenant-123",
		xctx.KeyTenantName:      "TestTenant",
		xctx.KeyTraceID:         "trace-xyz",
		xctx.KeySpanID:          "span-456",
		xctx.KeyRequestID:       "req-789",
		xctx.KeyHasParent:       "true",
		xctx.KeyUnclassRegionID: "region-001",
		xctx.KeyDeploymentType:  "LOCAL",
	}

	for key, want := range expected {
		if got := attrMap[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLogAttrs_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	if len(xctx.IdentityAttrs(nilCtx)) != 0 {
		t.Errorf("IdentityAttrs(nil) should return empty")
	}
	if len(xctx.TraceAttrs(nilCtx)) != 0 {
		t.Errorf("TraceAttrs(nil) should return empty")
	}
	if len(xctx.PlatformAttrs(nilCtx)) != 0 {
		t.Errorf("PlatformAttrs(nil) should return empty")
	}
	_, err := xctx.LogAttrs(nilCtx)
	if !errors.Is(err, xctx.ErrNilContext) {
		t.Errorf("LogAttrs(nil) error = %v, want %v", err, xctx.ErrNilContext)
	}
}

// =============================================================================
// Key 常量测试
// =============================================================================

func TestKeyConstants(t *testing.T) {
	t.Parallel()

	// Identity keys
	if xctx.KeyPlatformID != "platform_id" {
		t.Errorf("KeyPlatformID = %q, want %q", xctx.KeyPlatformID, "platform_id")
	}
	if xctx.KeyTenantID != "tenant_id" {
		t.Errorf("KeyTenantID = %q, want %q", xctx.KeyTenantID, "tenant_id")
	}
	if xctx.KeyTenantName != "tenant_name" {
		t.Errorf("KeyTenantName = %q, want %q", xctx.KeyTenantName, "tenant_name")
	}

	// Trace keys
	if xctx.KeyTraceID != "trace_id" {
		t.Errorf("KeyTraceID = %q, want %q", xctx.KeyTraceID, "trace_id")
	}
	if xctx.KeySpanID != "span_id" {
		t.Errorf("KeySpanID = %q, want %q", xctx.KeySpanID, "span_id")
	}
	if xctx.KeyRequestID != "request_id" {
		t.Errorf("KeyRequestID = %q, want %q", xctx.KeyRequestID, "request_id")
	}
	if xctx.KeyTraceFlags != "trace_flags" {
		t.Errorf("KeyTraceFlags = %q, want %q", xctx.KeyTraceFlags, "trace_flags")
	}

	// Platform keys
	if xctx.KeyHasParent != "has_parent" {
		t.Errorf("KeyHasParent = %q, want %q", xctx.KeyHasParent, "has_parent")
	}
	if xctx.KeyUnclassRegionID != "unclass_region_id" {
		t.Errorf("KeyUnclassRegionID = %q, want %q", xctx.KeyUnclassRegionID, "unclass_region_id")
	}
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleLogAttrs() {
	ctx, _ := xctx.WithPlatformID(context.Background(), "platform-abc")
	ctx, _ = xctx.WithTraceID(ctx, "trace-xyz")
	ctx, _ = xctx.WithRequestID(ctx, "req-123")
	ctx, err := xctx.WithDeploymentType(ctx, xctx.DeploymentSaaS)
	if err != nil {
		return
	}

	attrs, err := xctx.LogAttrs(ctx)
	if err != nil {
		return
	}
	for _, attr := range attrs {
		fmt.Printf("%s=%s\n", attr.Key, attr.Value.String())
	}
	// Output:
	// platform_id=platform-abc
	// trace_id=trace-xyz
	// request_id=req-123
	// deployment_type=SAAS
}
