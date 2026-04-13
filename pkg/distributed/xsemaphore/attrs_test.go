package xsemaphore

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAttrPermitID(t *testing.T) {
	attr := AttrPermitID("test-id")
	assert.Equal(t, attrKeyPermitID, attr.Key)
	assert.Equal(t, "test-id", attr.Value.String())
}

func TestAttrResource(t *testing.T) {
	attr := AttrResource("test-resource")
	assert.Equal(t, attrKeyResource, attr.Key)
	assert.Equal(t, "test-resource", attr.Value.String())
}

func TestAttrTenantID(t *testing.T) {
	attr := AttrTenantID("tenant-123")
	assert.Equal(t, attrKeyTenantID, attr.Key)
	assert.Equal(t, "tenant-123", attr.Value.String())
}

func TestAttrCapacity(t *testing.T) {
	attr := AttrCapacity(100)
	assert.Equal(t, attrKeyCapacity, attr.Key)
	assert.Equal(t, int64(100), attr.Value.Int64())
}

func TestAttrUsed(t *testing.T) {
	attr := AttrUsed(50)
	assert.Equal(t, attrKeyUsed, attr.Key)
	assert.Equal(t, int64(50), attr.Value.Int64())
}

func TestAttrAvailable(t *testing.T) {
	attr := AttrAvailable(50)
	assert.Equal(t, attrKeyAvailable, attr.Key)
	assert.Equal(t, int64(50), attr.Value.Int64())
}

func TestAttrError(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		err := errors.New("test error")
		attr := AttrError(err)
		assert.Equal(t, attrKeyError, attr.Key)
		assert.Equal(t, "test error", attr.Value.String())
	})

	t.Run("nil error returns zero Attr", func(t *testing.T) {
		attr := AttrError(nil)
		assert.Equal(t, "", attr.Key, "nil error should return zero-value Attr (elided by slog handlers)")
	})
}

func TestAttrReason(t *testing.T) {
	attr := AttrReason("capacity_full")
	assert.Equal(t, attrKeyReason, attr.Key)
	assert.Equal(t, "capacity_full", attr.Value.String())
}

func TestAttrDuration(t *testing.T) {
	d := 5 * time.Second
	attr := AttrDuration(d)
	assert.Equal(t, attrKeyDuration, attr.Key)
	assert.Equal(t, d, attr.Value.Duration())
}

func TestAttrGlobalCount(t *testing.T) {
	attr := AttrGlobalCount(10)
	assert.Equal(t, attrKeyGlobalCount, attr.Key)
	assert.Equal(t, int64(10), attr.Value.Int64())
}

func TestAttrTenantCount(t *testing.T) {
	attr := AttrTenantCount(5)
	assert.Equal(t, attrKeyTenantCount, attr.Key)
	assert.Equal(t, int64(5), attr.Value.Int64())
}

func TestAttrRetry(t *testing.T) {
	attr := AttrRetry(3)
	assert.Equal(t, attrKeyRetry, attr.Key)
	assert.Equal(t, int64(3), attr.Value.Int64())
}

func TestAttrMaxRetries(t *testing.T) {
	attr := AttrMaxRetries(10)
	assert.Equal(t, attrKeyMaxRetries, attr.Key)
	assert.Equal(t, int64(10), attr.Value.Int64())
}

func TestAttrSemType(t *testing.T) {
	attr := AttrSemType(SemaphoreTypeDistributed)
	assert.Equal(t, attrKeySemType, attr.Key)
	assert.Equal(t, SemaphoreTypeDistributed, attr.Value.String())
}

func TestAttrStrategy(t *testing.T) {
	attr := AttrStrategy(FallbackLocal)
	assert.Equal(t, attrKeyStrategy, attr.Key)
	assert.Equal(t, string(FallbackLocal), attr.Value.String())
}
