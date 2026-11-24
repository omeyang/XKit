package xauth

import (
	"testing"
)

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
		MetricsOpGetPlatformData,
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
		MetricsAttrHTTPPath,
		MetricsAttrHTTPMethod,
		MetricsAttrHTTPStatus,
	}
	for _, attr := range attrs {
		if attr == "" {
			t.Error("attribute key should not be empty")
		}
	}
}
