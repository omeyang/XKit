package xtenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"
	"github.com/omeyang/xkit/pkg/context/xtenant"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Trace Header 注入测试（覆盖 injectTraceHeaders 函数）
// =============================================================================

func TestInjectTraceHeaders_EmptyTrace(t *testing.T) {
	// 测试空 context 时 trace headers 不被设置
	xplatform.Reset() // 确保平台未初始化，避免干扰

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// 所有 trace header 应该为空
	if got := req.Header.Get(xtenant.HeaderTraceID); got != "" {
		t.Errorf("HeaderTraceID should be empty, got %q", got)
	}
	if got := req.Header.Get(xtenant.HeaderSpanID); got != "" {
		t.Errorf("HeaderSpanID should be empty, got %q", got)
	}
	if got := req.Header.Get(xtenant.HeaderRequestID); got != "" {
		t.Errorf("HeaderRequestID should be empty, got %q", got)
	}
	if got := req.Header.Get(xtenant.HeaderTraceFlags); got != "" {
		t.Errorf("HeaderTraceFlags should be empty, got %q", got)
	}
}

func TestInjectTraceHeaders_PartialTrace(t *testing.T) {
	// 测试只有部分 trace 字段时的行为
	xplatform.Reset()

	tests := []struct {
		name      string
		setupCtx  func(t *testing.T) context.Context
		wantTrace string
		wantSpan  string
		wantReq   string
		wantFlags string
	}{
		{
			name: "只有TraceID",
			setupCtx: func(t *testing.T) context.Context {
				ctx, err := xctx.WithTraceID(context.Background(), "trace-001")
				if err != nil {
					t.Fatalf("WithTraceID error: %v", err)
				}
				return ctx
			},
			wantTrace: "trace-001",
			wantSpan:  "",
			wantReq:   "",
			wantFlags: "",
		},
		{
			name: "TraceID和SpanID",
			setupCtx: func(t *testing.T) context.Context {
				ctx, err := xctx.WithTraceID(context.Background(), "trace-002")
				if err != nil {
					t.Fatalf("WithTraceID error: %v", err)
				}
				ctx, err = xctx.WithSpanID(ctx, "span-002")
				if err != nil {
					t.Fatalf("WithSpanID error: %v", err)
				}
				return ctx
			},
			wantTrace: "trace-002",
			wantSpan:  "span-002",
			wantReq:   "",
			wantFlags: "",
		},
		{
			name: "只有TraceFlags",
			setupCtx: func(t *testing.T) context.Context {
				ctx, err := xctx.WithTraceFlags(context.Background(), "01")
				if err != nil {
					t.Fatalf("WithTraceFlags error: %v", err)
				}
				return ctx
			},
			wantTrace: "",
			wantSpan:  "",
			wantReq:   "",
			wantFlags: "01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx(t)
			req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
			xtenant.InjectToRequest(ctx, req)

			if got := req.Header.Get(xtenant.HeaderTraceID); got != tt.wantTrace {
				t.Errorf("HeaderTraceID = %q, want %q", got, tt.wantTrace)
			}
			if got := req.Header.Get(xtenant.HeaderSpanID); got != tt.wantSpan {
				t.Errorf("HeaderSpanID = %q, want %q", got, tt.wantSpan)
			}
			if got := req.Header.Get(xtenant.HeaderRequestID); got != tt.wantReq {
				t.Errorf("HeaderRequestID = %q, want %q", got, tt.wantReq)
			}
			if got := req.Header.Get(xtenant.HeaderTraceFlags); got != tt.wantFlags {
				t.Errorf("HeaderTraceFlags = %q, want %q", got, tt.wantFlags)
			}
		})
	}
}

func TestInjectTraceHeaders_FullTrace(t *testing.T) {
	// 测试完整 trace 字段时的行为
	xplatform.Reset()

	ctx := context.Background()
	var err error

	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	if err != nil {
		t.Fatalf("WithTraceID error: %v", err)
	}
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	if err != nil {
		t.Fatalf("WithSpanID error: %v", err)
	}
	ctx, err = xctx.WithRequestID(ctx, "550e8400e29b41d4a716446655440000")
	if err != nil {
		t.Fatalf("WithRequestID error: %v", err)
	}
	ctx, err = xctx.WithTraceFlags(ctx, "01")
	if err != nil {
		t.Fatalf("WithTraceFlags error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// 验证所有 trace header 都被正确注入
	if got := req.Header.Get(xtenant.HeaderTraceID); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("HeaderTraceID = %q, want %q", got, "0af7651916cd43dd8448eb211c80319c")
	}
	if got := req.Header.Get(xtenant.HeaderSpanID); got != "b7ad6b7169203331" {
		t.Errorf("HeaderSpanID = %q, want %q", got, "b7ad6b7169203331")
	}
	if got := req.Header.Get(xtenant.HeaderRequestID); got != "550e8400e29b41d4a716446655440000" {
		t.Errorf("HeaderRequestID = %q, want %q", got, "550e8400e29b41d4a716446655440000")
	}
	if got := req.Header.Get(xtenant.HeaderTraceFlags); got != "01" {
		t.Errorf("HeaderTraceFlags = %q, want %q", got, "01")
	}
}

// =============================================================================
// Trace Header 提取测试
// =============================================================================

func TestExtractTraceFromHTTPHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    xctx.Trace
	}{
		{
			name:    "nil Header",
			headers: nil,
			want:    xctx.Trace{},
		},
		{
			name:    "空Header",
			headers: map[string]string{},
			want:    xctx.Trace{},
		},
		{
			name: "完整Trace",
			headers: map[string]string{
				xtenant.HeaderTraceID:    "trace-123",
				xtenant.HeaderSpanID:     "span-456",
				xtenant.HeaderRequestID:  "req-789",
				xtenant.HeaderTraceFlags: "01",
			},
			want: xctx.Trace{
				TraceID:    "trace-123",
				SpanID:     "span-456",
				RequestID:  "req-789",
				TraceFlags: "01",
			},
		},
		{
			name: "部分Trace",
			headers: map[string]string{
				xtenant.HeaderTraceID: "trace-only",
			},
			want: xctx.Trace{
				TraceID: "trace-only",
			},
		},
		{
			name: "带空白的值会被trim",
			headers: map[string]string{
				xtenant.HeaderTraceID:    "  trace-123  ",
				xtenant.HeaderSpanID:     "  span-456  ",
				xtenant.HeaderRequestID:  "  req-789  ",
				xtenant.HeaderTraceFlags: "  01  ",
			},
			want: xctx.Trace{
				TraceID:    "trace-123",
				SpanID:     "span-456",
				RequestID:  "req-789",
				TraceFlags: "01",
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

			got := xtenant.ExtractTraceFromHTTPHeader(h)
			if got.TraceID != tt.want.TraceID {
				t.Errorf("TraceID = %q, want %q", got.TraceID, tt.want.TraceID)
			}
			if got.SpanID != tt.want.SpanID {
				t.Errorf("SpanID = %q, want %q", got.SpanID, tt.want.SpanID)
			}
			if got.RequestID != tt.want.RequestID {
				t.Errorf("RequestID = %q, want %q", got.RequestID, tt.want.RequestID)
			}
			if got.TraceFlags != tt.want.TraceFlags {
				t.Errorf("TraceFlags = %q, want %q", got.TraceFlags, tt.want.TraceFlags)
			}
		})
	}
}

func TestExtractTraceFromHTTPRequest(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		got := xtenant.ExtractTraceFromHTTPRequest(nil)
		if got.TraceID != "" || got.SpanID != "" || got.RequestID != "" || got.TraceFlags != "" {
			t.Errorf("ExtractTraceFromHTTPRequest(nil) should return empty, got %+v", got)
		}
	})

	t.Run("有效请求", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTraceID, "t1")
		req.Header.Set(xtenant.HeaderSpanID, "s1")

		got := xtenant.ExtractTraceFromHTTPRequest(req)
		if got.TraceID != "t1" || got.SpanID != "s1" {
			t.Errorf("ExtractTraceFromHTTPRequest() = %+v, want TraceID=t1, SpanID=s1", got)
		}
	})
}

// =============================================================================
// WithRequireTenantID 选项测试
// =============================================================================

func TestHTTPMiddlewareWithOptions_RequireTenantID(t *testing.T) {
	t.Run("缺少TenantID返回400", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenantID(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		// 不设置任何租户信息
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("只有TenantID时正常通过", func(t *testing.T) {
		var capturedTenantID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTenantID = xtenant.TenantID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenantID(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
		// 不设置 TenantName - 应该允许通过

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if capturedTenantID != "tenant-123" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-123")
		}
	})

	t.Run("WithRequireTenant和WithRequireTenantID互斥", func(t *testing.T) {
		// 后设置的选项应该覆盖前面的
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// 先设置 RequireTenant，再设置 RequireTenantID
		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenant(),
			xtenant.WithRequireTenantID(), // 后设置，应该覆盖前面的
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
		// 不设置 TenantName - RequireTenantID 模式下应该允许通过

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d (后设置的 RequireTenantID 应该生效)", rr.Code, http.StatusOK)
		}
	})
}

// =============================================================================
// WithEnsureTrace 选项测试
// =============================================================================

// assertHTTPTraceFieldsGenerated 验证 HTTP 中间件中追踪字段均被自动生成（非空）。
func assertHTTPTraceFieldsGenerated(t *testing.T, traceID, spanID, requestID string) {
	t.Helper()
	assert.NotEmpty(t, traceID, "TraceID should be auto-generated")
	assert.NotEmpty(t, spanID, "SpanID should be auto-generated")
	assert.NotEmpty(t, requestID, "RequestID should be auto-generated")
}

// assertHTTPTraceFieldsEmpty 验证 HTTP 中间件中追踪字段均为空。
func assertHTTPTraceFieldsEmpty(t *testing.T, traceID, spanID, requestID string) {
	t.Helper()
	assert.Empty(t, traceID, "TraceID should be empty without EnsureTrace")
	assert.Empty(t, spanID, "SpanID should be empty without EnsureTrace")
	assert.Empty(t, requestID, "RequestID should be empty without EnsureTrace")
}

func TestHTTPMiddlewareWithOptions_EnsureTrace(t *testing.T) {
	t.Run("启用EnsureTrace自动生成追踪信息", func(t *testing.T) {
		var capturedTraceID, capturedSpanID, capturedRequestID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xctx.TraceID(r.Context())
			capturedSpanID = xctx.SpanID(r.Context())
			capturedRequestID = xctx.RequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithEnsureTrace(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assertHTTPTraceFieldsGenerated(t, capturedTraceID, capturedSpanID, capturedRequestID)
	})

	t.Run("启用EnsureTrace但上游已有trace则保留", func(t *testing.T) {
		const existingTraceID = "0af7651916cd43dd8448eb211c80319c"
		var capturedTraceID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xctx.TraceID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithEnsureTrace(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTraceID, existingTraceID)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, existingTraceID, capturedTraceID, "should preserve upstream TraceID")
	})

	t.Run("默认不启用EnsureTrace则不自动生成", func(t *testing.T) {
		var capturedTraceID, capturedSpanID, capturedRequestID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xctx.TraceID(r.Context())
			capturedSpanID = xctx.SpanID(r.Context())
			capturedRequestID = xctx.RequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assertHTTPTraceFieldsEmpty(t, capturedTraceID, capturedSpanID, capturedRequestID)
	})

	t.Run("上游传递trace则正常传播", func(t *testing.T) {
		const (
			existingTraceID    = "trace-upstream"
			existingSpanID     = "span-upstream"
			existingRequestID  = "req-upstream"
			existingTraceFlags = "01"
		)
		var capturedTraceID, capturedSpanID, capturedRequestID, capturedTraceFlags string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xctx.TraceID(r.Context())
			capturedSpanID = xctx.SpanID(r.Context())
			capturedRequestID = xctx.RequestID(r.Context())
			capturedTraceFlags = xctx.TraceFlags(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTraceID, existingTraceID)
		req.Header.Set(xtenant.HeaderSpanID, existingSpanID)
		req.Header.Set(xtenant.HeaderRequestID, existingRequestID)
		req.Header.Set(xtenant.HeaderTraceFlags, existingTraceFlags)

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, existingTraceID, capturedTraceID)
		assert.Equal(t, existingSpanID, capturedSpanID)
		assert.Equal(t, existingRequestID, capturedRequestID)
		assert.Equal(t, existingTraceFlags, capturedTraceFlags)
	})
}

// =============================================================================
// 组合选项测试
// =============================================================================

func TestHTTPMiddlewareWithOptions_Combined(t *testing.T) {
	t.Run("同时启用RequireTenantID和EnsureTrace", func(t *testing.T) {
		var capturedTenantID, capturedTraceID string

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTenantID = xtenant.TenantID(r.Context())
			capturedTraceID = xctx.TraceID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenantID(),
			xtenant.WithEnsureTrace(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
		// 不设置 trace header，应该自动生成

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if capturedTenantID != "tenant-123" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-123")
		}
		if capturedTraceID == "" {
			t.Error("TraceID should be auto-generated, got empty")
		}
	})

	t.Run("RequireTenantID失败时不应执行EnsureTrace", func(t *testing.T) {
		handlerCalled := false

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		wrapped := xtenant.HTTPMiddlewareWithOptions(
			xtenant.WithRequireTenantID(),
			xtenant.WithEnsureTrace(),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		// 不设置 TenantID，应该被拒绝

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
		if handlerCalled {
			t.Error("handler should not be called when tenant validation fails")
		}
	})
}

// =============================================================================
// FG-S2: 出站传播清理旧租户键测试
// =============================================================================

func TestInjectToRequest_ClearsStalePlatformHeaders(t *testing.T) {
	// FG-S1 回归测试：xplatform 未初始化时清除旧平台 Header
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID:      "plat-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})
	if err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
	xtenant.InjectToRequest(ctx, req)

	// 验证平台信息已注入
	assert.Equal(t, "plat-001", req.Header.Get(xtenant.HeaderPlatformID))
	assert.Equal(t, "true", req.Header.Get(xtenant.HeaderHasParent))
	assert.Equal(t, "region-001", req.Header.Get(xtenant.HeaderUnclassRegionID))

	// Reset xplatform，模拟请求对象复用但平台未初始化的场景
	xplatform.Reset()
	xtenant.InjectToRequest(ctx, req)

	assert.Empty(t, req.Header.Get(xtenant.HeaderPlatformID), "stale PlatformID should be cleared")
	assert.Empty(t, req.Header.Get(xtenant.HeaderHasParent), "stale HasParent should be cleared")
	assert.Empty(t, req.Header.Get(xtenant.HeaderUnclassRegionID), "stale UnclassRegionID should be cleared")
}

func TestInjectToRequest_ClearsStaleHeaders(t *testing.T) {
	xplatform.Reset()

	t.Run("清除旧租户Header", func(t *testing.T) {
		// 第一次调用: 注入租户信息
		ctx1 := context.Background()
		var err error
		ctx1, err = xctx.WithTenantID(ctx1, "tenant-old")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}
		ctx1, err = xctx.WithTenantName(ctx1, "OldTenant")
		if err != nil {
			t.Fatalf("WithTenantName() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
		xtenant.InjectToRequest(ctx1, req)

		// 验证第一次注入
		assert.Equal(t, "tenant-old", req.Header.Get(xtenant.HeaderTenantID))
		assert.Equal(t, "OldTenant", req.Header.Get(xtenant.HeaderTenantName))

		// 第二次调用: context 无租户信息，旧 Header 应被清除
		ctx2 := context.Background()
		xtenant.InjectToRequest(ctx2, req)

		assert.Empty(t, req.Header.Get(xtenant.HeaderTenantID), "stale TenantID should be cleared")
		assert.Empty(t, req.Header.Get(xtenant.HeaderTenantName), "stale TenantName should be cleared")
	})

	t.Run("清除旧Trace Header", func(t *testing.T) {
		ctx1 := context.Background()
		var err error
		ctx1, err = xctx.WithTraceID(ctx1, "trace-old")
		if err != nil {
			t.Fatalf("WithTraceID() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/downstream", nil)
		xtenant.InjectToRequest(ctx1, req)

		assert.Equal(t, "trace-old", req.Header.Get(xtenant.HeaderTraceID))

		// 第二次调用: context 无 trace 信息
		ctx2 := context.Background()
		xtenant.InjectToRequest(ctx2, req)

		assert.Empty(t, req.Header.Get(xtenant.HeaderTraceID), "stale TraceID should be cleared")
		assert.Empty(t, req.Header.Get(xtenant.HeaderSpanID), "stale SpanID should be cleared")
		assert.Empty(t, req.Header.Get(xtenant.HeaderRequestID), "stale RequestID should be cleared")
		assert.Empty(t, req.Header.Get(xtenant.HeaderTraceFlags), "stale TraceFlags should be cleared")
	})
}
