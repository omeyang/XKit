package xtenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"
	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// =============================================================================
// HTTP Header 提取测试
// =============================================================================

func TestExtractFromHTTPHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    xtenant.TenantInfo
	}{
		{
			name:    "空Header",
			headers: nil,
			want:    xtenant.TenantInfo{},
		},
		{
			name: "完整Header",
			headers: map[string]string{
				xtenant.HeaderTenantID:   "tenant-123",
				xtenant.HeaderTenantName: "TestTenant",
			},
			want: xtenant.TenantInfo{
				TenantID:   "tenant-123",
				TenantName: "TestTenant",
			},
		},
		{
			name: "只有TenantID",
			headers: map[string]string{
				xtenant.HeaderTenantID: "tenant-123",
			},
			want: xtenant.TenantInfo{
				TenantID: "tenant-123",
			},
		},
		{
			name: "带空白的值会被trim",
			headers: map[string]string{
				xtenant.HeaderTenantID:   "  tenant-123  ",
				xtenant.HeaderTenantName: "  TestTenant  ",
			},
			want: xtenant.TenantInfo{
				TenantID:   "tenant-123",
				TenantName: "TestTenant",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h http.Header
			if tt.headers != nil {
				h = make(http.Header)
				for k, v := range tt.headers {
					h.Set(k, v)
				}
			}

			got := xtenant.ExtractFromHTTPHeader(h)
			if got.TenantID != tt.want.TenantID || got.TenantName != tt.want.TenantName {
				t.Errorf("ExtractFromHTTPHeader() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestExtractFromHTTPHeader_MultiValue 验证多值 Header 取首值的契约（FG-L1 回归测试）。
//
// doc.go:31 定义了"单值字段，多值时取第一个"的语义。
func TestExtractFromHTTPHeader_MultiValue(t *testing.T) {
	h := make(http.Header)
	// http.Header.Add 追加多值
	h.Add(xtenant.HeaderTenantID, "first-tenant")
	h.Add(xtenant.HeaderTenantID, "second-tenant")
	h.Add(xtenant.HeaderTenantName, "first-name")
	h.Add(xtenant.HeaderTenantName, "second-name")

	info := xtenant.ExtractFromHTTPHeader(h)

	// 应取第一个值
	if info.TenantID != "first-tenant" {
		t.Errorf("TenantID = %q, want %q (should take first value)", info.TenantID, "first-tenant")
	}
	if info.TenantName != "first-name" {
		t.Errorf("TenantName = %q, want %q (should take first value)", info.TenantName, "first-name")
	}
}

func TestExtractFromHTTPRequest(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		got := xtenant.ExtractFromHTTPRequest(nil)
		if !got.IsEmpty() {
			t.Errorf("ExtractFromHTTPRequest(nil) should be empty, got %+v", got)
		}
	})

	t.Run("有效请求", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "t1")
		req.Header.Set(xtenant.HeaderTenantName, "n1")

		got := xtenant.ExtractFromHTTPRequest(req)
		if got.TenantID != "t1" || got.TenantName != "n1" {
			t.Errorf("ExtractFromHTTPRequest() = %+v, want TenantID=t1, TenantName=n1", got)
		}
	})
}

// =============================================================================
// HTTP 中间件测试
// =============================================================================

func TestHTTPMiddleware(t *testing.T) {
	t.Run("提取并注入租户信息", func(t *testing.T) {
		var capturedTenantID, capturedTenantName string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTenantID = xtenant.TenantID(r.Context())
			capturedTenantName = xtenant.TenantName(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
		req.Header.Set(xtenant.HeaderTenantName, "TestTenant")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if capturedTenantID != "tenant-123" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-123")
		}
		if capturedTenantName != "TestTenant" {
			t.Errorf("TenantName = %q, want %q", capturedTenantName, "TestTenant")
		}
	})

	t.Run("空Header正常通过", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})
}

func TestHTTPMiddlewareWithOptions_RequireTenant(t *testing.T) {
	t.Run("缺少租户信息返回400", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenant(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("有完整租户信息正常通过", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenant(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "t1")
		req.Header.Set(xtenant.HeaderTenantName, "n1")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})
}

// =============================================================================
// HTTP Header 注入测试
// =============================================================================

func TestInjectToRequest(t *testing.T) {
	t.Run("nil request不panic", func(t *testing.T) {
		ctx, err := xctx.WithTenantID(context.Background(), "t1")
		if err != nil {
			t.Fatalf("xctx.WithTenantID() error = %v", err)
		}
		xtenant.InjectToRequest(ctx, nil) // 不应该 panic
	})

	t.Run("注入租户信息", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantID(ctx, "tenant-123")
		if err != nil {
			t.Fatalf("xctx.WithTenantID() error = %v", err)
		}
		ctx, err = xctx.WithTenantName(ctx, "TestTenant")
		if err != nil {
			t.Fatalf("xctx.WithTenantName() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
		xtenant.InjectToRequest(ctx, req)

		if got := req.Header.Get(xtenant.HeaderTenantID); got != "tenant-123" {
			t.Errorf("HeaderTenantID = %q, want %q", got, "tenant-123")
		}
		if got := req.Header.Get(xtenant.HeaderTenantName); got != "TestTenant" {
			t.Errorf("HeaderTenantName = %q, want %q", got, "TestTenant")
		}
	})
}

func TestInjectTenantToHeader(t *testing.T) {
	t.Run("nil header不panic", func(t *testing.T) {
		info := xtenant.TenantInfo{TenantID: "t1"}
		xtenant.InjectTenantToHeader(nil, info) // 不应该 panic
	})

	t.Run("注入非空字段", func(t *testing.T) {
		h := make(http.Header)
		info := xtenant.TenantInfo{
			TenantID:   "t1",
			TenantName: "n1",
		}
		xtenant.InjectTenantToHeader(h, info)

		if got := h.Get(xtenant.HeaderTenantID); got != "t1" {
			t.Errorf("HeaderTenantID = %q, want %q", got, "t1")
		}
		if got := h.Get(xtenant.HeaderTenantName); got != "n1" {
			t.Errorf("HeaderTenantName = %q, want %q", got, "n1")
		}
	})

	t.Run("空字段不注入", func(t *testing.T) {
		h := make(http.Header)
		info := xtenant.TenantInfo{TenantID: "t1"} // TenantName 为空
		xtenant.InjectTenantToHeader(h, info)

		if got := h.Get(xtenant.HeaderTenantName); got != "" {
			t.Errorf("HeaderTenantName should be empty, got %q", got)
		}
	})

	t.Run("TrimSpace归一化（FG-M4回归测试）", func(t *testing.T) {
		h := make(http.Header)
		info := xtenant.TenantInfo{
			TenantID:   "  t1  ",
			TenantName: "  n1  ",
		}
		xtenant.InjectTenantToHeader(h, info)

		if got := h.Get(xtenant.HeaderTenantID); got != "t1" {
			t.Errorf("HeaderTenantID = %q, want %q (should TrimSpace)", got, "t1")
		}
		if got := h.Get(xtenant.HeaderTenantName); got != "n1" {
			t.Errorf("HeaderTenantName = %q, want %q (should TrimSpace)", got, "n1")
		}
	})

	t.Run("纯空白值不注入（FG-M4回归测试）", func(t *testing.T) {
		h := make(http.Header)
		info := xtenant.TenantInfo{
			TenantID:   "   ",
			TenantName: "  \t",
		}
		xtenant.InjectTenantToHeader(h, info)

		if got := h.Get(xtenant.HeaderTenantID); got != "" {
			t.Errorf("HeaderTenantID should be empty for whitespace-only, got %q", got)
		}
		if got := h.Get(xtenant.HeaderTenantName); got != "" {
			t.Errorf("HeaderTenantName should be empty for whitespace-only, got %q", got)
		}
	})
}

// =============================================================================
// nil 选项守卫测试（FG-L3 回归测试）
// =============================================================================

func TestHTTPMiddlewareWithOptions_NilOption(t *testing.T) {
	// nil 选项不应该 panic
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := xtenant.HTTPMiddlewareWithOptions(
		nil, // nil 选项
		xtenant.WithRequireTenantID(),
		nil, // 另一个 nil 选项
	)(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(xtenant.HeaderTenantID, "t1")
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// =============================================================================
// 补充测试：覆盖更多边界情况
// =============================================================================

func TestHTTPMiddlewareWithOptions_OnlyTenantID(t *testing.T) {
	t.Run("只有TenantID时正常通过", func(t *testing.T) {
		var capturedTenantID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTenantID = xtenant.TenantID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-only")
		// 不设置 TenantName

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if capturedTenantID != "tenant-only" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-only")
		}
	})
}

func TestHTTPMiddlewareWithOptions_OnlyTenantName(t *testing.T) {
	t.Run("只有TenantName时正常通过", func(t *testing.T) {
		var capturedTenantName string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTenantName = xtenant.TenantName(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantName, "name-only")
		// 不设置 TenantID

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if capturedTenantName != "name-only" {
			t.Errorf("TenantName = %q, want %q", capturedTenantName, "name-only")
		}
	})
}

func TestInjectToRequest_OnlyTenantID(t *testing.T) {
	ctx := context.Background()
	ctx, err := xctx.WithTenantID(ctx, "tenant-only")
	if err != nil {
		t.Fatalf("xctx.WithTenantID() error = %v", err)
	}
	// 不设置 TenantName

	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	if got := req.Header.Get(xtenant.HeaderTenantID); got != "tenant-only" {
		t.Errorf("HeaderTenantID = %q, want %q", got, "tenant-only")
	}
	// TenantName 应该为空
	if got := req.Header.Get(xtenant.HeaderTenantName); got != "" {
		t.Errorf("HeaderTenantName = %q, want empty", got)
	}
}

func TestInjectToRequest_OnlyTenantName(t *testing.T) {
	ctx := context.Background()
	ctx, err := xctx.WithTenantName(ctx, "name-only")
	if err != nil {
		t.Fatalf("xctx.WithTenantName() error = %v", err)
	}
	// 不设置 TenantID

	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// TenantID 应该为空
	if got := req.Header.Get(xtenant.HeaderTenantID); got != "" {
		t.Errorf("HeaderTenantID = %q, want empty", got)
	}
	if got := req.Header.Get(xtenant.HeaderTenantName); got != "name-only" {
		t.Errorf("HeaderTenantName = %q, want %q", got, "name-only")
	}
}

func TestHTTPMiddlewareWithOptions_RequireTenant_MissingTenantName(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := xtenant.HTTPMiddlewareWithOptions(
		xtenant.WithRequireTenant(),
	)(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(xtenant.HeaderTenantID, "t1") // 只有 ID，没有 Name

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// 应该返回 400，因为缺少 TenantName
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// xplatform 集成测试（覆盖平台信息注入分支）
// =============================================================================

func TestInjectToRequest_WithPlatformInitialized(t *testing.T) {
	// 初始化 xplatform
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID:      "test-platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})
	if err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}
	t.Cleanup(xplatform.Reset)

	// 设置租户信息
	ctx := context.Background()
	ctx, err = xctx.WithTenantID(ctx, "tenant-123")
	if err != nil {
		t.Fatalf("xctx.WithTenantID() error = %v", err)
	}
	ctx, err = xctx.WithTenantName(ctx, "TestTenant")
	if err != nil {
		t.Fatalf("xctx.WithTenantName() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// 验证平台信息 Header
	if got := req.Header.Get(xtenant.HeaderPlatformID); got != "test-platform-001" {
		t.Errorf("HeaderPlatformID = %q, want %q", got, "test-platform-001")
	}
	if got := req.Header.Get(xtenant.HeaderHasParent); got != "true" {
		t.Errorf("HeaderHasParent = %q, want %q", got, "true")
	}
	if got := req.Header.Get(xtenant.HeaderUnclassRegionID); got != "region-001" {
		t.Errorf("HeaderUnclassRegionID = %q, want %q", got, "region-001")
	}

	// 验证租户信息 Header
	if got := req.Header.Get(xtenant.HeaderTenantID); got != "tenant-123" {
		t.Errorf("HeaderTenantID = %q, want %q", got, "tenant-123")
	}
	if got := req.Header.Get(xtenant.HeaderTenantName); got != "TestTenant" {
		t.Errorf("HeaderTenantName = %q, want %q", got, "TestTenant")
	}
}

func TestInjectToRequest_WithPlatformNoParent(t *testing.T) {
	// 初始化 xplatform，HasParent = false
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID: "test-platform-002",
		HasParent:  false,
	})
	if err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}
	t.Cleanup(xplatform.Reset)

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// 验证 HasParent = false
	if got := req.Header.Get(xtenant.HeaderHasParent); got != "false" {
		t.Errorf("HeaderHasParent = %q, want %q", got, "false")
	}
	// UnclassRegionID 为空时不应设置 Header
	if got := req.Header.Get(xtenant.HeaderUnclassRegionID); got != "" {
		t.Errorf("HeaderUnclassRegionID should be empty, got %q", got)
	}
}

func TestInjectToRequest_WithPlatformNotInitialized(t *testing.T) {
	// 确保 xplatform 未初始化
	xplatform.Reset()

	ctx := context.Background()
	ctx, err := xctx.WithTenantID(ctx, "tenant-123")
	if err != nil {
		t.Fatalf("xctx.WithTenantID() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// xplatform 未初始化时不应设置平台相关 Header
	if got := req.Header.Get(xtenant.HeaderPlatformID); got != "" {
		t.Errorf("HeaderPlatformID should be empty when xplatform not initialized, got %q", got)
	}
	if got := req.Header.Get(xtenant.HeaderHasParent); got != "" {
		t.Errorf("HeaderHasParent should be empty when xplatform not initialized, got %q", got)
	}
	if got := req.Header.Get(xtenant.HeaderUnclassRegionID); got != "" {
		t.Errorf("HeaderUnclassRegionID should be empty when xplatform not initialized, got %q", got)
	}
	// 但租户信息应该正常设置
	if got := req.Header.Get(xtenant.HeaderTenantID); got != "tenant-123" {
		t.Errorf("HeaderTenantID = %q, want %q", got, "tenant-123")
	}
}

// TestInjectToRequest_PlatformSnapshotConsistency 回归测试：
// 验证 injectPlatformHeaders 单次快照语义，Reset/Init 并发下
// 每次注入结果保持内部一致（PlatformID 与 HasParent/UnclassRegionID 来自同一配置，
// 不会出现 PlatformID 来自 Init 前、HasParent 来自 Init 后的撕裂）。
func TestInjectToRequest_PlatformSnapshotConsistency(t *testing.T) {
	xplatform.Reset()
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "snapshot-platform",
		HasParent:       true,
		UnclassRegionID: "snapshot-region",
	}); err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}
	t.Cleanup(xplatform.Reset)

	var stop atomic.Bool
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			xplatform.Reset()
			if err := xplatform.Init(xplatform.Config{
				PlatformID:      "snapshot-platform",
				HasParent:       true,
				UnclassRegionID: "snapshot-region",
			}); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		stop.Store(true)
		wg.Wait()
	})

	const iters = 200
	for range iters {
		req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
		xtenant.InjectToRequest(context.Background(), req)
		pid := req.Header.Get(xtenant.HeaderPlatformID)
		hp := req.Header.Get(xtenant.HeaderHasParent)
		rid := req.Header.Get(xtenant.HeaderUnclassRegionID)

		// 内部一致性：要么三者全来自 "snapshot-platform" 快照，要么全部为空（未初始化快照）。
		allSet := pid == "snapshot-platform" && hp == "true" && rid == "snapshot-region"
		allEmpty := pid == "" && hp == "" && rid == ""
		if !allSet && !allEmpty {
			t.Fatalf("torn platform snapshot: platform=%q hasParent=%q region=%q", pid, hp, rid)
		}
	}
}
