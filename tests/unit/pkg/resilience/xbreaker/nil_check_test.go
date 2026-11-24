package xbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNilInterfaceValue(t *testing.T) {
	t.Run("PlainNil", func(t *testing.T) {
		assert.True(t, isNilInterfaceValue(nil))
	})

	t.Run("TypedNilPointer", func(t *testing.T) {
		var p *ConsecutiveFailuresPolicy
		assert.True(t, isNilInterfaceValue(p))
	})

	t.Run("NonNilPointer", func(t *testing.T) {
		p := NewConsecutiveFailures(3)
		assert.False(t, isNilInterfaceValue(p))
	})

	t.Run("NonNilableType", func(t *testing.T) {
		assert.False(t, isNilInterfaceValue(42))
	})
}
