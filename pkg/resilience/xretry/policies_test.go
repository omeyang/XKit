package xretry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixedRetryPolicy(t *testing.T) {
	t.Run("MaxAttempts", func(t *testing.T) {
		tests := []struct {
			input    int
			expected int
		}{
			{3, 3},
			{1, 1},
			{0, 1},  // 最小值为 1
			{-1, 1}, // 负值也设为 1
			{100, 100},
		}

		for _, tt := range tests {
			p := NewFixedRetry(tt.input)
			assert.Equal(t, tt.expected, p.MaxAttempts())
		}
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		p := NewFixedRetry(3)
		ctx := context.Background()
		err := errors.New("test error")

		// 前两次应该重试
		assert.True(t, p.ShouldRetry(ctx, 1, err))
		assert.True(t, p.ShouldRetry(ctx, 2, err))

		// 第三次不应该重试（已达到最大次数）
		assert.False(t, p.ShouldRetry(ctx, 3, err))

		// 超过最大次数
		assert.False(t, p.ShouldRetry(ctx, 4, err))
	})

	t.Run("ShouldRetry_ContextCanceled", func(t *testing.T) {
		p := NewFixedRetry(3)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assert.False(t, p.ShouldRetry(ctx, 1, errors.New("test")))
	})

	t.Run("ShouldRetry_PermanentError", func(t *testing.T) {
		p := NewFixedRetry(3)
		ctx := context.Background()
		err := NewPermanentError(errors.New("permanent"))

		assert.False(t, p.ShouldRetry(ctx, 1, err))
	})
}

func TestAlwaysRetryPolicy(t *testing.T) {
	p := NewAlwaysRetry()

	t.Run("MaxAttempts", func(t *testing.T) {
		assert.Equal(t, 0, p.MaxAttempts())
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		ctx := context.Background()
		err := errors.New("test error")

		// 应该一直重试
		for i := 1; i <= 100; i++ {
			assert.True(t, p.ShouldRetry(ctx, i, err))
		}
	})

	t.Run("ShouldRetry_ContextCanceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assert.False(t, p.ShouldRetry(ctx, 1, errors.New("test")))
	})

	t.Run("ShouldRetry_PermanentError", func(t *testing.T) {
		ctx := context.Background()
		err := NewPermanentError(errors.New("permanent"))

		assert.False(t, p.ShouldRetry(ctx, 1, err))
	})
}

func TestNeverRetryPolicy(t *testing.T) {
	p := NewNeverRetry()

	t.Run("MaxAttempts", func(t *testing.T) {
		assert.Equal(t, 1, p.MaxAttempts())
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		ctx := context.Background()
		err := errors.New("test error")

		// 永不重试
		for i := 1; i <= 10; i++ {
			assert.False(t, p.ShouldRetry(ctx, i, err))
		}
	})
}
