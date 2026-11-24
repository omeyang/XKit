package xmongo

import (
	"context"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/stretchr/testify/assert"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	assert.Equal(t, 5*time.Second, opts.HealthTimeout)
	assert.Equal(t, time.Duration(0), opts.SlowQueryThreshold)
	assert.Nil(t, opts.SlowQueryHook)
	assert.NotNil(t, opts.Observer)
}

func TestWithHealthTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{
			name:     "设置 1 秒超时",
			timeout:  time.Second,
			expected: time.Second,
		},
		{
			name:     "设置 30 秒超时",
			timeout:  30 * time.Second,
			expected: 30 * time.Second,
		},
		{
			name:     "设置零值应保持默认值",
			timeout:  0,
			expected: 5 * time.Second,
		},
		{
			name:     "设置负值应保持默认值",
			timeout:  -1 * time.Second,
			expected: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			WithHealthTimeout(tt.timeout)(opts)
			assert.Equal(t, tt.expected, opts.HealthTimeout)
		})
	}
}

func TestWithSlowQueryThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold time.Duration
		expected  time.Duration
	}{
		{
			name:      "设置 100ms 阈值",
			threshold: 100 * time.Millisecond,
			expected:  100 * time.Millisecond,
		},
		{
			name:      "设置 1s 阈值",
			threshold: time.Second,
			expected:  time.Second,
		},
		{
			name:      "设置零值禁用慢查询",
			threshold: 0,
			expected:  0,
		},
		{
			name:      "负值被忽略保持默认值",
			threshold: -100 * time.Millisecond,
			expected:  0, // 默认值为 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			WithSlowQueryThreshold(tt.threshold)(opts)
			assert.Equal(t, tt.expected, opts.SlowQueryThreshold)
		})
	}
}

func TestWithSlowQueryHook(t *testing.T) {
	t.Run("设置慢查询钩子", func(t *testing.T) {
		var called bool
		hook := func(_ context.Context, _ SlowQueryInfo) {
			called = true
		}

		opts := defaultOptions()
		WithSlowQueryHook(hook)(opts)

		assert.NotNil(t, opts.SlowQueryHook)
		// 调用钩子验证是否正确设置
		opts.SlowQueryHook(context.Background(), SlowQueryInfo{})
		assert.True(t, called)
	})

	t.Run("设置 nil 钩子", func(t *testing.T) {
		opts := defaultOptions()
		WithSlowQueryHook(nil)(opts)
		assert.Nil(t, opts.SlowQueryHook)
	})
}

func TestSlowQueryInfo(t *testing.T) {
	info := SlowQueryInfo{
		Database:   "testdb",
		Collection: "users",
		Operation:  "find",
		Filter:     map[string]any{"name": "test"},
		Duration:   150 * time.Millisecond,
	}

	assert.Equal(t, "testdb", info.Database)
	assert.Equal(t, "users", info.Collection)
	assert.Equal(t, "find", info.Operation)
	assert.Equal(t, map[string]any{"name": "test"}, info.Filter)
	assert.Equal(t, 150*time.Millisecond, info.Duration)
}

func TestWithObserver(t *testing.T) {
	opts := defaultOptions()
	observer := xmetrics.NoopObserver{}

	WithObserver(observer)(opts)

	assert.Equal(t, observer, opts.Observer)
}

func TestWithObserver_Nil(t *testing.T) {
	opts := defaultOptions()
	original := opts.Observer

	WithObserver(nil)(opts)

	assert.Equal(t, original, opts.Observer)
}

func TestOptionsChaining(t *testing.T) {
	var hookCalled bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		hookCalled = true
	}

	opts := defaultOptions()
	// 链式应用多个选项
	WithHealthTimeout(10 * time.Second)(opts)
	WithSlowQueryThreshold(200 * time.Millisecond)(opts)
	WithSlowQueryHook(hook)(opts)

	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
	assert.Equal(t, 200*time.Millisecond, opts.SlowQueryThreshold)
	assert.NotNil(t, opts.SlowQueryHook)

	opts.SlowQueryHook(context.Background(), SlowQueryInfo{})
	assert.True(t, hookCalled)
}
