package xretry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermanentError(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		err := NewPermanentError(errors.New("test error"))
		assert.Equal(t, "test error", err.Error())
	})

	t.Run("ErrorNil", func(t *testing.T) {
		err := NewPermanentError(nil)
		assert.Equal(t, "permanent error", err.Error())
	})

	t.Run("Unwrap", func(t *testing.T) {
		inner := errors.New("inner error")
		err := NewPermanentError(inner)
		assert.Equal(t, inner, err.Unwrap())
	})

	t.Run("Retryable", func(t *testing.T) {
		err := NewPermanentError(errors.New("test"))
		assert.False(t, err.Retryable())
	})
}

func TestTemporaryError(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		err := NewTemporaryError(errors.New("test error"))
		assert.Equal(t, "test error", err.Error())
	})

	t.Run("ErrorNil", func(t *testing.T) {
		err := NewTemporaryError(nil)
		assert.Equal(t, "temporary error", err.Error())
	})

	t.Run("Unwrap", func(t *testing.T) {
		inner := errors.New("inner error")
		err := NewTemporaryError(inner)
		assert.Equal(t, inner, err.Unwrap())
	})

	t.Run("Retryable", func(t *testing.T) {
		err := NewTemporaryError(errors.New("test"))
		assert.True(t, err.Retryable())
	})
}

func TestIsRetryable(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		assert.False(t, IsRetryable(nil))
	})

	t.Run("PermanentError", func(t *testing.T) {
		err := NewPermanentError(errors.New("test"))
		assert.False(t, IsRetryable(err))
	})

	t.Run("TemporaryError", func(t *testing.T) {
		err := NewTemporaryError(errors.New("test"))
		assert.True(t, IsRetryable(err))
	})

	t.Run("RegularError", func(t *testing.T) {
		err := errors.New("regular error")
		assert.True(t, IsRetryable(err)) // 默认可重试
	})

	t.Run("WrappedPermanentError", func(t *testing.T) {
		inner := NewPermanentError(errors.New("inner"))
		wrapped := errors.Join(errors.New("wrapper"), inner)
		assert.False(t, IsRetryable(wrapped))
	})

	t.Run("WrappedTemporaryError", func(t *testing.T) {
		inner := NewTemporaryError(errors.New("inner"))
		wrapped := errors.Join(errors.New("wrapper"), inner)
		assert.True(t, IsRetryable(wrapped))
	})
}

func TestIsPermanent(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		assert.False(t, IsPermanent(nil))
	})

	t.Run("PermanentError", func(t *testing.T) {
		err := NewPermanentError(errors.New("test"))
		assert.True(t, IsPermanent(err))
	})

	t.Run("TemporaryError", func(t *testing.T) {
		err := NewTemporaryError(errors.New("test"))
		assert.False(t, IsPermanent(err))
	})

	t.Run("RegularError", func(t *testing.T) {
		err := errors.New("regular error")
		assert.False(t, IsPermanent(err)) // 默认可重试
	})
}
