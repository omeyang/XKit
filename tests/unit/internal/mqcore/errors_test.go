package mqcore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Error Prefix Tests
// =============================================================================

func TestErrors_Prefix(t *testing.T) {
	errors := []error{
		ErrNilClient,
		ErrNilMessage,
		ErrNilHandler,
		ErrClosed,
	}

	for _, err := range errors {
		assert.True(t, strings.HasPrefix(err.Error(), "mq:"),
			"error %q should have 'mq:' prefix", err.Error())
	}
}

func TestErrNilClient(t *testing.T) {
	assert.Contains(t, ErrNilClient.Error(), "nil client")
}

func TestErrNilMessage(t *testing.T) {
	assert.Contains(t, ErrNilMessage.Error(), "nil message")
}

func TestErrNilHandler(t *testing.T) {
	assert.Contains(t, ErrNilHandler.Error(), "nil handler")
}

func TestErrClosed(t *testing.T) {
	assert.Contains(t, ErrClosed.Error(), "closed")
}
