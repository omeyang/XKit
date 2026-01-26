package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func TestTenantIDFromContext(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTenantID(ctx, "tenant-from-context")

		tenantID := TenantIDFromContext(ctx)
		if tenantID != "tenant-from-context" {
			t.Errorf("tenantID = %q, expected 'tenant-from-context'", tenantID)
		}
	})

	t.Run("empty context returns env", func(t *testing.T) {
		ctx := context.Background()

		// TenantIDFromContext will try env var, which may or may not be set
		// Just verify it doesn't panic
		_ = TenantIDFromContext(ctx)
	})
}

func TestAsContextClient(t *testing.T) {
	t.Run("real client implements ContextClient", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		cc := AsContextClient(c)
		if cc == nil {
			t.Error("real client should implement ContextClient")
		}
	})

	t.Run("mock client does not implement ContextClient", func(t *testing.T) {
		mc := newMockClient()
		cc := AsContextClient(mc)
		if cc != nil {
			t.Error("mock client should not implement ContextClient")
		}
	})
}

func TestClient_GetTokenFromContext(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"access_token": "context-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close()

	// Cast to ContextClient
	cc := AsContextClient(c)
	if cc == nil {
		t.Fatal("client should implement ContextClient")
	}

	t.Run("with tenant ID in context", func(t *testing.T) {
		ctxWithTenant, _ := xctx.WithTenantID(ctx, "tenant-123")

		token, err := cc.GetTokenFromContext(ctxWithTenant)
		if err != nil {
			t.Fatalf("GetTokenFromContext failed: %v", err)
		}
		if token != "context-token" {
			t.Errorf("token = %q, expected 'context-token'", token)
		}
	})

	t.Run("without tenant ID in context", func(t *testing.T) {
		_, err := cc.GetTokenFromContext(ctx)
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_GetPlatformIDFromContext(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathPlatformSelf {
			resp := PlatformSelfResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: "platform-from-context"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Token endpoint
		resp := map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close()

	cc := AsContextClient(c)
	if cc == nil {
		t.Fatal("client should implement ContextClient")
	}

	t.Run("with tenant ID in context", func(t *testing.T) {
		ctxWithTenant, _ := xctx.WithTenantID(ctx, "tenant-123")

		platformID, err := cc.GetPlatformIDFromContext(ctxWithTenant)
		if err != nil {
			t.Fatalf("GetPlatformIDFromContext failed: %v", err)
		}
		if platformID != "platform-from-context" {
			t.Errorf("platformID = %q, expected 'platform-from-context'", platformID)
		}
	})

	t.Run("without tenant ID in context", func(t *testing.T) {
		_, err := cc.GetPlatformIDFromContext(ctx)
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_HasParentPlatformFromContext(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathHasParent {
			resp := HasParentResponse{Data: true}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Token endpoint
		resp := map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close()

	cc := AsContextClient(c)
	if cc == nil {
		t.Fatal("client should implement ContextClient")
	}

	t.Run("with tenant ID in context", func(t *testing.T) {
		ctxWithTenant, _ := xctx.WithTenantID(ctx, "tenant-123")

		hasParent, err := cc.HasParentPlatformFromContext(ctxWithTenant)
		if err != nil {
			t.Fatalf("HasParentPlatformFromContext failed: %v", err)
		}
		if !hasParent {
			t.Error("expected hasParent to be true")
		}
	})

	t.Run("without tenant ID in context", func(t *testing.T) {
		_, err := cc.HasParentPlatformFromContext(ctx)
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_GetUnclassRegionIDFromContext(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathUnclassRegion {
			resp := UnclassRegionResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: "region-from-context"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Token endpoint
		resp := map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close()

	cc := AsContextClient(c)
	if cc == nil {
		t.Fatal("client should implement ContextClient")
	}

	t.Run("with tenant ID in context", func(t *testing.T) {
		ctxWithTenant, _ := xctx.WithTenantID(ctx, "tenant-123")

		regionID, err := cc.GetUnclassRegionIDFromContext(ctxWithTenant)
		if err != nil {
			t.Fatalf("GetUnclassRegionIDFromContext failed: %v", err)
		}
		if regionID != "region-from-context" {
			t.Errorf("regionID = %q, expected 'region-from-context'", regionID)
		}
	})

	t.Run("without tenant ID in context", func(t *testing.T) {
		_, err := cc.GetUnclassRegionIDFromContext(ctx)
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestWithPlatformInfo(t *testing.T) {
	ctx := context.Background()

	t.Run("successful injection", func(t *testing.T) {
		mc := newMockClient()
		mc.platformIDs["tenant-123"] = "platform-456"

		newCtx, err := WithPlatformInfo(ctx, mc, "tenant-123")
		if err != nil {
			t.Fatalf("WithPlatformInfo failed: %v", err)
		}

		// Verify platform ID was injected
		platformID := xctx.PlatformID(newCtx)
		if platformID != "platform-456" {
			t.Errorf("platformID = %q, expected 'platform-456'", platformID)
		}

		// Verify tenant ID was injected
		tenantID := xctx.TenantID(newCtx)
		if tenantID != "tenant-123" {
			t.Errorf("tenantID = %q, expected 'tenant-123'", tenantID)
		}
	})

	t.Run("empty tenantID resolved from context", func(t *testing.T) {
		mc := newMockClient()
		mc.platformIDs["context-tenant"] = "platform-from-context"

		// Set tenant ID in context
		ctxWithTenant, _ := xctx.WithTenantID(ctx, "context-tenant")

		// Pass empty tenantID - should be resolved from context
		newCtx, err := WithPlatformInfo(ctxWithTenant, mc, "")
		if err != nil {
			t.Fatalf("WithPlatformInfo failed: %v", err)
		}

		// Verify platform ID was injected using resolved tenant ID
		platformID := xctx.PlatformID(newCtx)
		if platformID != "platform-from-context" {
			t.Errorf("platformID = %q, expected 'platform-from-context'", platformID)
		}

		// Verify the resolved tenant ID was written to context
		tenantID := xctx.TenantID(newCtx)
		if tenantID != "context-tenant" {
			t.Errorf("tenantID = %q, expected 'context-tenant'", tenantID)
		}
	})

	t.Run("empty tenantID without context returns error", func(t *testing.T) {
		mc := newMockClient()

		// Pass empty tenantID without context - should return error
		_, err := WithPlatformInfo(ctx, mc, "")
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})

	t.Run("error getting platform ID", func(t *testing.T) {
		mc := newMockClient()
		mc.getPlatformIDErr = ErrPlatformIDNotFound

		_, err := WithPlatformInfo(ctx, mc, "tenant-123")
		if err != ErrPlatformIDNotFound {
			t.Errorf("expected ErrPlatformIDNotFound, got %v", err)
		}
	})
}
