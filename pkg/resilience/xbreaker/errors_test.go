package xbreaker

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOpen(t *testing.T) {
	t.Run("ErrOpenState", func(t *testing.T) {
		assert.True(t, IsOpen(ErrOpenState))
	})

	t.Run("wrapped ErrOpenState", func(t *testing.T) {
		wrapped := fmt.Errorf("operation failed: %w", ErrOpenState)
		assert.True(t, IsOpen(wrapped))
	})

	t.Run("other error", func(t *testing.T) {
		assert.False(t, IsOpen(errors.New("some error")))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsOpen(nil))
	})
}

func TestIsTooManyRequests(t *testing.T) {
	t.Run("ErrTooManyRequests", func(t *testing.T) {
		assert.True(t, IsTooManyRequests(ErrTooManyRequests))
	})

	t.Run("wrapped ErrTooManyRequests", func(t *testing.T) {
		wrapped := fmt.Errorf("rate limited: %w", ErrTooManyRequests)
		assert.True(t, IsTooManyRequests(wrapped))
	})

	t.Run("other error", func(t *testing.T) {
		assert.False(t, IsTooManyRequests(errors.New("some error")))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsTooManyRequests(nil))
	})
}

func TestIsBreakerError(t *testing.T) {
	t.Run("ErrOpenState", func(t *testing.T) {
		assert.True(t, IsBreakerError(ErrOpenState))
	})

	t.Run("ErrTooManyRequests", func(t *testing.T) {
		assert.True(t, IsBreakerError(ErrTooManyRequests))
	})

	t.Run("wrapped ErrOpenState", func(t *testing.T) {
		wrapped := fmt.Errorf("operation failed: %w", ErrOpenState)
		assert.True(t, IsBreakerError(wrapped))
	})

	t.Run("wrapped ErrTooManyRequests", func(t *testing.T) {
		wrapped := fmt.Errorf("rate limited: %w", ErrTooManyRequests)
		assert.True(t, IsBreakerError(wrapped))
	})

	t.Run("other error", func(t *testing.T) {
		assert.False(t, IsBreakerError(errors.New("some error")))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsBreakerError(nil))
	})
}

func TestIsRecoverable(t *testing.T) {
	t.Run("ErrOpenState is recoverable", func(t *testing.T) {
		assert.True(t, IsRecoverable(ErrOpenState))
	})

	t.Run("ErrTooManyRequests is recoverable", func(t *testing.T) {
		assert.True(t, IsRecoverable(ErrTooManyRequests))
	})

	t.Run("other error is not recoverable", func(t *testing.T) {
		assert.False(t, IsRecoverable(errors.New("business error")))
	})

	t.Run("nil error is not recoverable", func(t *testing.T) {
		assert.False(t, IsRecoverable(nil))
	})
}
