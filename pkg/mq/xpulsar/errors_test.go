package xpulsar

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
	assert.Same(t, mqcore.ErrNilClient, ErrNilClient)
	assert.Same(t, mqcore.ErrNilMessage, ErrNilMessage)
	assert.Same(t, mqcore.ErrNilHandler, ErrNilHandler)
}

// =============================================================================
// Pulsar Specific Error Tests
// =============================================================================

func TestErrEmptyURL(t *testing.T) {
	assert.True(t, strings.HasPrefix(ErrEmptyURL.Error(), "xpulsar:"),
		"error should have 'xpulsar:' prefix")
	assert.Contains(t, ErrEmptyURL.Error(), "empty URL")
}
