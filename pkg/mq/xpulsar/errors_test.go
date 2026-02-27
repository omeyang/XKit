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
	assert.Same(t, mqcore.ErrClosed, ErrClosed)
}

// =============================================================================
// Pulsar Specific Error Tests
// =============================================================================

func TestErrEmptyURL(t *testing.T) {
	assert.True(t, strings.HasPrefix(ErrEmptyURL.Error(), "xpulsar:"),
		"error should have 'xpulsar:' prefix")
	assert.Contains(t, ErrEmptyURL.Error(), "empty URL")
}

func TestPulsarSpecificErrors(t *testing.T) {
	errors := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNilOption", ErrNilOption, "nil option"},
		{"ErrNilProducer", ErrNilProducer, "nil producer"},
		{"ErrNilConsumer", ErrNilConsumer, "nil consumer"},
	}

	for _, tc := range errors {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, strings.HasPrefix(tc.err.Error(), "xpulsar:"),
				"error should have 'xpulsar:' prefix")
			assert.Contains(t, tc.err.Error(), tc.want)
		})
	}
}

func TestErrClosed_IsShared(t *testing.T) {
	// ErrClosed 现在是共享错误，使用 mq: 前缀而非 xpulsar: 前缀
	assert.Contains(t, ErrClosed.Error(), "closed")
}
