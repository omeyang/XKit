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

func TestBreakerError_Error(t *testing.T) {
	t.Run("with name", func(t *testing.T) {
		be := &BreakerError{Err: ErrOpenState, Name: "my-svc", State: StateOpen}
		assert.Equal(t, "breaker my-svc: circuit breaker is open", be.Error())
	})

	t.Run("without name", func(t *testing.T) {
		be := &BreakerError{Err: ErrOpenState, State: StateOpen}
		assert.Equal(t, "circuit breaker is open", be.Error())
	})
}

func TestWrapBreakerError_AlreadyWrapped(t *testing.T) {
	original := &BreakerError{Err: ErrOpenState, Name: "inner", State: StateOpen}
	wrapped := wrapBreakerError(original, "outer", StateClosed)
	// 应保留原始 BreakerError，不重复包装
	var be *BreakerError
	assert.True(t, errors.As(wrapped, &be))
	assert.Equal(t, "inner", be.Name)
}

func TestErrFailedByPolicy(t *testing.T) {
	assert.EqualError(t, errFailedByPolicy, "xbreaker: operation marked as failed by success policy")
}
