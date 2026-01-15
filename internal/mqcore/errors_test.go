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
		ErrNilConfig,
		ErrNilClient,
		ErrNilMessage,
		ErrNilHandler,
		ErrClosed,
		ErrHealthCheckFailed,
		ErrDLQPolicyRequired,
		ErrDLQTopicRequired,
		ErrRetryPolicyRequired,
	}

	for _, err := range errors {
		assert.True(t, strings.HasPrefix(err.Error(), "mq:"),
			"error %q should have 'mq:' prefix", err.Error())
	}
}

func TestErrNilConfig(t *testing.T) {
	assert.Contains(t, ErrNilConfig.Error(), "nil config")
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

func TestErrHealthCheckFailed(t *testing.T) {
	assert.Contains(t, ErrHealthCheckFailed.Error(), "health check")
}

func TestErrDLQPolicyRequired(t *testing.T) {
	assert.Contains(t, ErrDLQPolicyRequired.Error(), "DLQ policy")
}

func TestErrDLQTopicRequired(t *testing.T) {
	assert.Contains(t, ErrDLQTopicRequired.Error(), "DLQ topic")
}

func TestErrRetryPolicyRequired(t *testing.T) {
	assert.Contains(t, ErrRetryPolicyRequired.Error(), "retry policy")
}
