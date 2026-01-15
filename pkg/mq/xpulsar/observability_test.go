package xpulsar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// pulsarAttrs Tests
// =============================================================================

func TestPulsarAttrs_WithTopic(t *testing.T) {
	attrs := pulsarAttrs("test-topic")

	assert.Len(t, attrs, 2)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "pulsar", attrs[0].Value.(string)) //nolint:errcheck // 测试断言类型已知
	assert.Equal(t, "messaging.destination", attrs[1].Key)
	assert.Equal(t, "test-topic", attrs[1].Value.(string)) //nolint:errcheck // 测试断言类型已知
}

func TestPulsarAttrs_EmptyTopic(t *testing.T) {
	attrs := pulsarAttrs("")

	assert.Len(t, attrs, 1)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "pulsar", attrs[0].Value.(string)) //nolint:errcheck // 测试断言类型已知
}

func TestComponentName(t *testing.T) {
	assert.Equal(t, "xpulsar", componentName)
}
