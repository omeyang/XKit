package xlimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPKeyExtractor_Default(t *testing.T) {
	extractor := DefaultHTTPKeyExtractor()

	req := httptest.NewRequest(http.MethodGet, "/v1/users/123", nil)
	req.Header.Set("X-Tenant-ID", "tenant-001")
	req.Header.Set("X-Caller-ID", "order-service")

	key := extractor.Extract(req)

	if key.Tenant != "tenant-001" {
		t.Errorf("expected tenant 'tenant-001', got %q", key.Tenant)
	}
	if key.Caller != "order-service" {
		t.Errorf("expected caller 'order-service', got %q", key.Caller)
	}
	if key.Method != http.MethodGet {
		t.Errorf("expected method 'GET', got %q", key.Method)
	}
	if key.Path != "/v1/users/123" {
		t.Errorf("expected path '/v1/users/123', got %q", key.Path)
	}
}

func TestHTTPKeyExtractor_CustomHeaders(t *testing.T) {
	extractor := NewHTTPKeyExtractor(
		WithTenantHeader("X-Custom-Tenant"),
		WithCallerHeader("X-Custom-Caller"),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/orders", nil)
	req.Header.Set("X-Custom-Tenant", "custom-tenant")
	req.Header.Set("X-Custom-Caller", "custom-caller")

	key := extractor.Extract(req)

	if key.Tenant != "custom-tenant" {
		t.Errorf("expected tenant 'custom-tenant', got %q", key.Tenant)
	}
	if key.Caller != "custom-caller" {
		t.Errorf("expected caller 'custom-caller', got %q", key.Caller)
	}
}

func TestHTTPKeyExtractor_MissingHeaders(t *testing.T) {
	extractor := DefaultHTTPKeyExtractor()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	// 不设置任何 header

	key := extractor.Extract(req)

	if key.Tenant != "" {
		t.Errorf("expected empty tenant, got %q", key.Tenant)
	}
	if key.Caller != "" {
		t.Errorf("expected empty caller, got %q", key.Caller)
	}
	// Method 和 Path 始终存在
	if key.Method != http.MethodGet {
		t.Errorf("expected method 'GET', got %q", key.Method)
	}
	if key.Path != "/api/health" {
		t.Errorf("expected path '/api/health', got %q", key.Path)
	}
}

func TestHTTPKeyExtractor_WithPathNormalizer(t *testing.T) {
	// 路径规范化器：将 /v1/users/123 转换为 /v1/users/:id
	normalizer := func(path string) string {
		// 简单示例：将数字 ID 替换为 :id
		result := make([]byte, 0, len(path))
		i := 0
		for i < len(path) {
			if path[i] >= '0' && path[i] <= '9' {
				result = append(result, ":id"...)
				for i < len(path) && path[i] >= '0' && path[i] <= '9' {
					i++
				}
			} else {
				result = append(result, path[i])
				i++
			}
		}
		return string(result)
	}

	extractor := NewHTTPKeyExtractor(
		WithPathNormalizer(normalizer),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/users/12345", nil)
	key := extractor.Extract(req)

	if key.Path != "/v:id/users/:id" {
		t.Errorf("expected normalized path '/v:id/users/:id', got %q", key.Path)
	}
}

func TestHTTPKeyExtractor_WithExtra(t *testing.T) {
	// 自定义额外信息提取
	extractor := NewHTTPKeyExtractor(
		WithExtraExtractor(func(r *http.Request) map[string]string {
			return map[string]string{
				"region":  r.Header.Get("X-Region"),
				"version": r.Header.Get("X-API-Version"),
			}
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-Region", "us-east-1")
	req.Header.Set("X-API-Version", "v2")

	key := extractor.Extract(req)

	if key.Extra["region"] != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got %q", key.Extra["region"])
	}
	if key.Extra["version"] != "v2" {
		t.Errorf("expected version 'v2', got %q", key.Extra["version"])
	}
}

func TestHTTPKeyExtractor_WithResource(t *testing.T) {
	// 从 URL 提取资源
	extractor := NewHTTPKeyExtractor(
		WithResourceExtractor(func(r *http.Request) string {
			// 从路径提取资源类型，例如 /v1/users -> users
			path := r.URL.Path
			if len(path) > 1 {
				parts := splitPath(path)
				if len(parts) >= 2 {
					return parts[1] // 返回第二段，如 users
				}
			}
			return ""
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/orders/123", nil)
	key := extractor.Extract(req)

	if key.Resource != "orders" {
		t.Errorf("expected resource 'orders', got %q", key.Resource)
	}
}

// splitPath 辅助函数
func splitPath(path string) []string {
	var parts []string
	var current []byte
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if len(current) > 0 {
				parts = append(parts, string(current))
				current = current[:0]
			}
		} else {
			current = append(current, path[i])
		}
	}
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	return parts
}

func TestHTTPKeyExtractor_NilRequest(t *testing.T) {
	extractor := DefaultHTTPKeyExtractor()

	// Extract 应该能处理 nil 请求
	key := extractor.Extract(nil)

	// 应该返回空 Key，不 panic
	if key.Tenant != "" || key.Method != "" {
		t.Error("nil request should return empty key")
	}
}
