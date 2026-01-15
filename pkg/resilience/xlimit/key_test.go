package xlimit

import (
	"testing"
)

func TestKey_Render(t *testing.T) {
	tests := []struct {
		name     string
		key      Key
		template string
		want     string
	}{
		{
			name: "tenant only",
			key: Key{
				Tenant: "abc123",
			},
			template: "tenant:${tenant_id}",
			want:     "tenant:abc123",
		},
		{
			name: "tenant and API",
			key: Key{
				Tenant: "abc123",
				Method: "POST",
				Path:   "/v1/users",
			},
			template: "tenant:${tenant_id}:api:${method}:${path}",
			want:     "tenant:abc123:api:POST:/v1/users",
		},
		{
			name: "caller and resource",
			key: Key{
				Caller:   "order-service",
				Resource: "createOrder",
			},
			template: "caller:${caller_id}:resource:${resource}",
			want:     "caller:order-service:resource:createOrder",
		},
		{
			name: "all fields",
			key: Key{
				Tenant:   "tenant1",
				Caller:   "caller1",
				Method:   "GET",
				Path:     "/api/v1/items",
				Resource: "listItems",
			},
			template: "${tenant_id}:${caller_id}:${method}:${path}:${resource}",
			want:     "tenant1:caller1:GET:/api/v1/items:listItems",
		},
		{
			name: "extra fields",
			key: Key{
				Tenant: "abc123",
				Extra: map[string]string{
					"region": "us-east-1",
					"tier":   "premium",
				},
			},
			template: "tenant:${tenant_id}:region:${region}:tier:${tier}",
			want:     "tenant:abc123:region:us-east-1:tier:premium",
		},
		{
			name: "missing field renders as empty",
			key: Key{
				Tenant: "abc123",
			},
			template: "tenant:${tenant_id}:caller:${caller_id}",
			want:     "tenant:abc123:caller:",
		},
		{
			name:     "empty key renders empty values",
			key:      Key{},
			template: "tenant:${tenant_id}",
			want:     "tenant:",
		},
		{
			name:     "no variables",
			key:      Key{Tenant: "abc"},
			template: "static-key",
			want:     "static-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.Render(tt.template)
			if got != tt.want {
				t.Errorf("Key.Render(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestKey_String(t *testing.T) {
	tests := []struct {
		name string
		key  Key
		want string
	}{
		{
			name: "tenant only",
			key:  Key{Tenant: "abc123"},
			want: "tenant=abc123",
		},
		{
			name: "tenant and caller",
			key:  Key{Tenant: "abc123", Caller: "order-service"},
			want: "tenant=abc123,caller=order-service",
		},
		{
			name: "all basic fields",
			key: Key{
				Tenant:   "t1",
				Caller:   "c1",
				Method:   "POST",
				Path:     "/api",
				Resource: "r1",
			},
			want: "tenant=t1,caller=c1,method=POST,path=/api,resource=r1",
		},
		{
			name: "empty key",
			key:  Key{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.String()
			if got != tt.want {
				t.Errorf("Key.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKey_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		key  Key
		want bool
	}{
		{
			name: "empty key",
			key:  Key{},
			want: true,
		},
		{
			name: "tenant set",
			key:  Key{Tenant: "abc"},
			want: false,
		},
		{
			name: "caller set",
			key:  Key{Caller: "svc"},
			want: false,
		},
		{
			name: "method set",
			key:  Key{Method: "GET"},
			want: false,
		},
		{
			name: "path set",
			key:  Key{Path: "/api"},
			want: false,
		},
		{
			name: "resource set",
			key:  Key{Resource: "r1"},
			want: false,
		},
		{
			name: "extra set",
			key:  Key{Extra: map[string]string{"k": "v"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.IsEmpty(); got != tt.want {
				t.Errorf("Key.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKey_WithTenant(t *testing.T) {
	key := Key{}.WithTenant("abc123")
	if key.Tenant != "abc123" {
		t.Errorf("WithTenant() Tenant = %q, want %q", key.Tenant, "abc123")
	}
}

func TestKey_WithCaller(t *testing.T) {
	key := Key{}.WithCaller("order-service")
	if key.Caller != "order-service" {
		t.Errorf("WithCaller() Caller = %q, want %q", key.Caller, "order-service")
	}
}

func TestKey_WithMethod(t *testing.T) {
	key := Key{}.WithMethod("POST")
	if key.Method != "POST" {
		t.Errorf("WithMethod() Method = %q, want %q", key.Method, "POST")
	}
}

func TestKey_WithPath(t *testing.T) {
	key := Key{}.WithPath("/v1/users")
	if key.Path != "/v1/users" {
		t.Errorf("WithPath() Path = %q, want %q", key.Path, "/v1/users")
	}
}

func TestKey_WithResource(t *testing.T) {
	key := Key{}.WithResource("createOrder")
	if key.Resource != "createOrder" {
		t.Errorf("WithResource() Resource = %q, want %q", key.Resource, "createOrder")
	}
}

func TestKey_WithExtra(t *testing.T) {
	key := Key{}.WithExtra("region", "us-east-1")
	if key.Extra["region"] != "us-east-1" {
		t.Errorf("WithExtra() Extra[region] = %q, want %q", key.Extra["region"], "us-east-1")
	}

	// 链式调用
	key = key.WithExtra("tier", "premium")
	if key.Extra["tier"] != "premium" {
		t.Errorf("WithExtra() Extra[tier] = %q, want %q", key.Extra["tier"], "premium")
	}
	// 验证之前的值还在
	if key.Extra["region"] != "us-east-1" {
		t.Errorf("WithExtra() should preserve previous values")
	}
}

func TestKey_Chaining(t *testing.T) {
	key := Key{}.
		WithTenant("t1").
		WithCaller("c1").
		WithMethod("POST").
		WithPath("/api").
		WithResource("r1").
		WithExtra("env", "prod")

	if key.Tenant != "t1" {
		t.Errorf("Tenant = %q, want %q", key.Tenant, "t1")
	}
	if key.Caller != "c1" {
		t.Errorf("Caller = %q, want %q", key.Caller, "c1")
	}
	if key.Method != "POST" {
		t.Errorf("Method = %q, want %q", key.Method, "POST")
	}
	if key.Path != "/api" {
		t.Errorf("Path = %q, want %q", key.Path, "/api")
	}
	if key.Resource != "r1" {
		t.Errorf("Resource = %q, want %q", key.Resource, "r1")
	}
	if key.Extra["env"] != "prod" {
		t.Errorf("Extra[env] = %q, want %q", key.Extra["env"], "prod")
	}
}

func BenchmarkKey_Render(b *testing.B) {
	key := Key{
		Tenant: "tenant123",
		Caller: "order-service",
		Method: "POST",
		Path:   "/v1/orders",
	}
	template := "tenant:${tenant_id}:caller:${caller_id}:api:${method}:${path}"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = key.Render(template)
	}
}

func BenchmarkKey_String(b *testing.B) {
	key := Key{
		Tenant:   "tenant123",
		Caller:   "order-service",
		Method:   "POST",
		Path:     "/v1/orders",
		Resource: "createOrder",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = key.String()
	}
}

func FuzzKey_Render(f *testing.F) {
	// 种子语料
	f.Add("tenant123", "order-service", "POST", "/v1/orders", "tenant:${tenant_id}")
	f.Add("abc", "svc", "GET", "/api", "${tenant_id}:${caller_id}:${method}:${path}")
	f.Add("", "", "", "", "static")
	f.Add("special:chars", "with/slash", "POST", "/path/with:colon", "${tenant_id}:${path}")

	f.Fuzz(func(t *testing.T, tenant, caller, method, path, template string) {
		key := Key{
			Tenant: tenant,
			Caller: caller,
			Method: method,
			Path:   path,
		}

		// 验证 Render 不会 panic
		_ = key.Render(template)
	})
}
