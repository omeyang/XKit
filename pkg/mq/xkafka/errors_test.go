package xkafka

import (
	"strings"
	"testing"

	"github.com/omeyang/xkit/internal/mqcore"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Shared Error Re-export Tests
// =============================================================================

func TestSharedErrors_AreAliases(t *testing.T) {
	// 验证重导出的共享错误是 mqcore 的别名
	assert.Same(t, mqcore.ErrNilClient, ErrNilClient)
	assert.Same(t, mqcore.ErrNilMessage, ErrNilMessage)
	assert.Same(t, mqcore.ErrNilHandler, ErrNilHandler)
	assert.Same(t, mqcore.ErrClosed, ErrClosed)
}

// =============================================================================
// Kafka Specific Error Tests
// =============================================================================

func TestKafkaSpecificErrors_Prefix(t *testing.T) {
	errors := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNilConfig", ErrNilConfig, "nil config"},
		{"ErrHealthCheckFailed", ErrHealthCheckFailed, "health check failed"},
		{"ErrDLQPolicyRequired", ErrDLQPolicyRequired, "DLQ policy"},
		{"ErrDLQTopicRequired", ErrDLQTopicRequired, "DLQ topic"},
		{"ErrRetryPolicyRequired", ErrRetryPolicyRequired, "retry policy"},
		{"ErrFlushTimeout", ErrFlushTimeout, "flush timeout"},
		{"ErrEmptyTopics", ErrEmptyTopics, "empty topics"},
	}

	for _, tc := range errors {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, strings.HasPrefix(tc.err.Error(), "xkafka:"),
				"error should have 'xkafka:' prefix")
			assert.Contains(t, tc.err.Error(), tc.want)
		})
	}
}
