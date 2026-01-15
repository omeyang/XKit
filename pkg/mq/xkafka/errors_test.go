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
	// 验证重导出的错误是 mqcore 的别名
	assert.Same(t, mqcore.ErrNilConfig, ErrNilConfig)
	assert.Same(t, mqcore.ErrNilClient, ErrNilClient)
	assert.Same(t, mqcore.ErrNilMessage, ErrNilMessage)
	assert.Same(t, mqcore.ErrNilHandler, ErrNilHandler)
	assert.Same(t, mqcore.ErrClosed, ErrClosed)
	assert.Same(t, mqcore.ErrHealthCheckFailed, ErrHealthCheckFailed)
	assert.Same(t, mqcore.ErrDLQPolicyRequired, ErrDLQPolicyRequired)
	assert.Same(t, mqcore.ErrDLQTopicRequired, ErrDLQTopicRequired)
	assert.Same(t, mqcore.ErrRetryPolicyRequired, ErrRetryPolicyRequired)
}

// =============================================================================
// Kafka Specific Error Tests
// =============================================================================

func TestErrNoPartitionAssigned(t *testing.T) {
	assert.True(t, strings.HasPrefix(ErrNoPartitionAssigned.Error(), "xkafka:"),
		"error should have 'xkafka:' prefix")
	assert.Contains(t, ErrNoPartitionAssigned.Error(), "partition")
}

func TestErrFlushTimeout(t *testing.T) {
	assert.True(t, strings.HasPrefix(ErrFlushTimeout.Error(), "xkafka:"),
		"error should have 'xkafka:' prefix")
	assert.Contains(t, ErrFlushTimeout.Error(), "flush timeout")
}

func TestErrEmptyTopics(t *testing.T) {
	assert.True(t, strings.HasPrefix(ErrEmptyTopics.Error(), "xkafka:"),
		"error should have 'xkafka:' prefix")
	assert.Contains(t, ErrEmptyTopics.Error(), "empty topics")
}
