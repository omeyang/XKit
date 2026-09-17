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
	assert.Equal(t, "pulsar", attrs[0].Value)
	assert.Equal(t, "messaging.destination.name", attrs[1].Key)
	assert.Equal(t, "test-topic", attrs[1].Value)
}

func TestPulsarAttrs_EmptyTopic(t *testing.T) {
	attrs := pulsarAttrs("")

	assert.Len(t, attrs, 1)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "pulsar", attrs[0].Value)
}

func TestComponentName(t *testing.T) {
	assert.Equal(t, "xpulsar", componentName)
}
