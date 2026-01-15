package xauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	observer := xmetrics.NoopObserver{}

	newCtx, span := startSpan(ctx, observer, "TestOperation",
		xmetrics.Attr{Key: "key1", Value: "value1"},
	)

	if newCtx == nil {
		t.Error("context should not be nil")
	}
	if span == nil {
		t.Error("span should not be nil")
	}

	// End span without error
	endSpan(span, nil)
}

func TestEndSpan(t *testing.T) {
	ctx := context.Background()
	observer := xmetrics.NoopObserver{}

	t.Run("without error", func(t *testing.T) {
		_, span := startSpan(ctx, observer, "TestOperation")
		endSpan(span, nil)
		// Should not panic
	})

	t.Run("with error", func(t *testing.T) {
		_, span := startSpan(ctx, observer, "TestOperation")
		endSpan(span, errors.New("test error"))
		// Should not panic
	})

	t.Run("with extra attributes", func(t *testing.T) {
		_, span := startSpan(ctx, observer, "TestOperation")
		endSpan(span, nil,
			xmetrics.Attr{Key: "extra1", Value: "value1"},
			xmetrics.Attr{Key: "extra2", Value: 123},
		)
		// Should not panic
	})
}

func TestNoopMetricsRecorder(t *testing.T) {
	ctx := context.Background()
	recorder := NoopMetricsRecorder{}

	// All methods should not panic
	t.Run("RecordTokenObtain", func(t *testing.T) {
		recorder.RecordTokenObtain(ctx, "tenant-1", "client_credentials", time.Second, nil)
		recorder.RecordTokenObtain(ctx, "tenant-1", "api_key", time.Second, errors.New("error"))
	})

	t.Run("RecordTokenVerify", func(t *testing.T) {
		recorder.RecordTokenVerify(ctx, time.Second, nil)
		recorder.RecordTokenVerify(ctx, time.Second, errors.New("error"))
	})

	t.Run("RecordTokenRefresh", func(t *testing.T) {
		recorder.RecordTokenRefresh(ctx, "tenant-1", time.Second, nil)
		recorder.RecordTokenRefresh(ctx, "tenant-1", time.Second, errors.New("error"))
	})

	t.Run("RecordCacheHit", func(t *testing.T) {
		recorder.RecordCacheHit(ctx, "GetToken", "tenant-1", true)
		recorder.RecordCacheHit(ctx, "GetToken", "tenant-1", false)
	})

	t.Run("RecordHTTPRequest", func(t *testing.T) {
		recorder.RecordHTTPRequest(ctx, "GET", "/api/test", 200, time.Second, nil)
		recorder.RecordHTTPRequest(ctx, "POST", "/api/test", 500, time.Second, errors.New("error"))
	})
}

func TestNewObserverMetricsRecorder(t *testing.T) {
	t.Run("with nil observer", func(t *testing.T) {
		recorder := NewObserverMetricsRecorder(nil)
		if recorder.observer == nil {
			t.Error("observer should default to NoopObserver")
		}
	})

	t.Run("with custom observer", func(t *testing.T) {
		observer := xmetrics.NoopObserver{}
		recorder := NewObserverMetricsRecorder(observer)
		// Should not panic and recorder should be created
		if recorder == nil {
			t.Error("recorder should not be nil")
		}
	})
}

func TestObserverMetricsRecorder(t *testing.T) {
	ctx := context.Background()
	observer := xmetrics.NoopObserver{}
	recorder := NewObserverMetricsRecorder(observer)

	// All methods should not panic
	t.Run("RecordTokenObtain", func(t *testing.T) {
		recorder.RecordTokenObtain(ctx, "tenant-1", "client_credentials", time.Second, nil)
		recorder.RecordTokenObtain(ctx, "tenant-1", "api_key", time.Second, errors.New("error"))
	})

	t.Run("RecordTokenVerify", func(t *testing.T) {
		recorder.RecordTokenVerify(ctx, time.Second, nil)
		recorder.RecordTokenVerify(ctx, time.Second, errors.New("error"))
	})

	t.Run("RecordTokenRefresh", func(t *testing.T) {
		recorder.RecordTokenRefresh(ctx, "tenant-1", time.Second, nil)
		recorder.RecordTokenRefresh(ctx, "tenant-1", time.Second, errors.New("error"))
	})

	t.Run("RecordCacheHit", func(t *testing.T) {
		recorder.RecordCacheHit(ctx, "GetToken", "tenant-1", true)
		recorder.RecordCacheHit(ctx, "GetToken", "tenant-1", false)
	})

	t.Run("RecordHTTPRequest", func(t *testing.T) {
		recorder.RecordHTTPRequest(ctx, "GET", "/api/test", 200, time.Second, nil)
		recorder.RecordHTTPRequest(ctx, "POST", "/api/test", 500, time.Second, errors.New("error"))
	})
}

func TestMetricsConstants(t *testing.T) {
	// Verify constants are defined
	if MetricsComponent != "xauth" {
		t.Errorf("MetricsComponent = %q, expected 'xauth'", MetricsComponent)
	}

	// Operation names
	operations := []string{
		MetricsOpGetToken,
		MetricsOpVerifyToken,
		MetricsOpRefreshToken,
		MetricsOpGetPlatformID,
		MetricsOpHasParentPlatform,
		MetricsOpGetUnclassRegion,
		MetricsOpHTTPRequest,
	}
	for _, op := range operations {
		if op == "" {
			t.Error("operation name should not be empty")
		}
	}

	// Attribute keys
	attrs := []string{
		MetricsAttrTenantID,
		MetricsAttrCacheHit,
		MetricsAttrTokenType,
		MetricsAttrPath,
		MetricsAttrMethod,
		MetricsAttrStatus,
	}
	for _, attr := range attrs {
		if attr == "" {
			t.Error("attribute key should not be empty")
		}
	}
}
