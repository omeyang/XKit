package xretry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsNilInterfaceValue(t *testing.T) {
	t.Run("PlainNil", func(t *testing.T) {
		assert.True(t, isNilInterfaceValue(nil))
	})

	t.Run("TypedNilPointer", func(t *testing.T) {
		var p *FixedRetryPolicy
		assert.True(t, isNilInterfaceValue(p))
	})

	t.Run("NonNilPointer", func(t *testing.T) {
		p := NewFixedRetry(3)
		assert.False(t, isNilInterfaceValue(p))
	})

	t.Run("TypedNilBackoffPolicy", func(t *testing.T) {
		var b *ExponentialBackoff
		assert.True(t, isNilInterfaceValue(b))
	})

	t.Run("NonNilableType", func(t *testing.T) {
		assert.False(t, isNilInterfaceValue(42))
	})
}

func TestWithRetryPolicy_TypedNil(t *testing.T) {
	var p *FixedRetryPolicy
	r := NewRetryer(WithRetryPolicy(p))

	// typed-nil 应被忽略，使用默认策略
	assert.NotNil(t, r.RetryPolicy())

	// 应能正常执行不 panic
	var attempts int
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("retry")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestWithBackoffPolicy_TypedNil(t *testing.T) {
	var b *ExponentialBackoff
	r := NewRetryer(WithBackoffPolicy(b))

	// typed-nil 应被忽略，使用默认策略
	assert.NotNil(t, r.BackoffPolicy())

	// 应能正常执行不 panic
	err := r.Do(context.Background(), func(_ context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

func TestBuildOptions_TypedNilPolicies(t *testing.T) {
	// 通过零值 Retryer 手动设置 typed-nil 验证 buildOptions 防御
	r := &Retryer{}
	var rp *FixedRetryPolicy
	var bp *FixedBackoff
	r.retryPolicy = rp
	r.backoffPolicy = bp

	// buildOptions 应回退到默认策略
	err := r.Do(context.Background(), func(_ context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

func TestNewRetryer_NilFunctionalOption(t *testing.T) {
	// 传入 nil 函数作为 option 不应 panic
	assert.NotPanics(t, func() {
		_ = NewRetryer(nil)
	})
}

func TestNewExponentialBackoff_NilOption(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = NewExponentialBackoff(nil)
	})
}

func TestToDelayType_TypedNil(t *testing.T) {
	var b *FixedBackoff
	fn := ToDelayType(b)

	// typed-nil 应返回零延迟函数
	delay := fn(1, nil, nil)
	assert.Equal(t, time.Duration(0), delay)
}

func TestNewRetrier_NilOption(t *testing.T) {
	// NewRetrier 传入 nil Option 不应 panic
	assert.NotPanics(t, func() {
		retrier := NewRetrier(nil)
		assert.NotNil(t, retrier)
	})
}

func TestNewRetrierWithData_NilOption(t *testing.T) {
	// NewRetrierWithData 传入 nil Option 不应 panic
	assert.NotPanics(t, func() {
		retrier := NewRetrierWithData[string](nil)
		assert.NotNil(t, retrier)
	})
}

func TestDo_NilOption(t *testing.T) {
	// Do 传入 nil Option 不应 panic
	assert.NotPanics(t, func() {
		err := Do(context.Background(), func() error { return nil }, nil)
		assert.NoError(t, err)
	})
}

func TestDoWithData_NilOption(t *testing.T) {
	// DoWithData 传入 nil Option 不应 panic
	assert.NotPanics(t, func() {
		result, err := DoWithData(context.Background(), func() (string, error) {
			return "ok", nil
		}, nil)
		assert.NoError(t, err)
		assert.Equal(t, "ok", result)
	})
}
