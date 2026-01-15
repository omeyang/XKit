//go:build e2e

package e2e

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xtenant"
	"github.com/omeyang/xkit/pkg/observability/xlog"
	"github.com/omeyang/xkit/pkg/observability/xtrace"
)

type captureHandler struct {
	mu    sync.Mutex
	attrs map[string]slog.Value
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Resolve()
		return true
	})

	h.mu.Lock()
	h.attrs = attrs
	h.mu.Unlock()

	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *captureHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *captureHandler) snapshot() map[string]slog.Value {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make(map[string]slog.Value, len(h.attrs))
	for k, v := range h.attrs {
		out[k] = v
	}
	return out
}

func TestHTTPTraceTenantChain_E2E(t *testing.T) {
	capture := &captureHandler{}
	logger := slog.New(xlog.NewEnrichHandler(capture))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "handled")
		w.WriteHeader(http.StatusOK)
	})

	wrapped := xtrace.HTTPMiddleware()(xtenant.HTTPMiddleware()(handler))

	req := httptest.NewRequest(http.MethodGet, "http://example/test", nil)
	req.Header.Set(xtrace.HeaderTraceID, "trace-123")
	req.Header.Set(xtrace.HeaderSpanID, "span-456")
	req.Header.Set(xtrace.HeaderRequestID, "req-789")
	req.Header.Set(xtenant.HeaderTenantID, "tenant-001")
	req.Header.Set(xtenant.HeaderTenantName, "tenant-name")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	attrs := capture.snapshot()
	assertAttr(t, attrs, xctx.KeyTraceID, "trace-123")
	assertAttr(t, attrs, xctx.KeySpanID, "span-456")
	assertAttr(t, attrs, xctx.KeyRequestID, "req-789")
	assertAttr(t, attrs, xctx.KeyTenantID, "tenant-001")
	assertAttr(t, attrs, xctx.KeyTenantName, "tenant-name")
}

func assertAttr(t *testing.T, attrs map[string]slog.Value, key, expected string) {
	t.Helper()
	val, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attr: %s", key)
	}
	if val.String() != expected {
		t.Fatalf("attr %s = %q, want %q", key, val.String(), expected)
	}
}
