package xclickhouse

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
		{"设置 1 秒超时", 1 * time.Second, 1 * time.Second},
		{"设置 30 秒超时", 30 * time.Second, 30 * time.Second},
		{"设置零值应保持原值", 0, 5 * time.Second},
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
		{"设置 100ms 阈值", 100 * time.Millisecond, 100 * time.Millisecond},
		{"设置 1s 阈值", 1 * time.Second, 1 * time.Second},
		{"设置零值禁用慢查询", 0, 0},
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
	tests := []struct {
		name    string
		hook    SlowQueryHook
		wantNil bool
	}{
		{
			"设置慢查询钩子",
			func(_ context.Context, _ SlowQueryInfo) {},
			false,
		},
		{
			"设置 nil 钩子",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			WithSlowQueryHook(tt.hook)(opts)
			if tt.wantNil {
				assert.Nil(t, opts.SlowQueryHook)
			} else {
				assert.NotNil(t, opts.SlowQueryHook)
			}
		})
	}
}

func TestSlowQueryInfo(t *testing.T) {
	info := SlowQueryInfo{
		Query:    "SELECT * FROM users WHERE id = ?",
		Args:     []any{1},
		Duration: 150 * time.Millisecond,
	}

	assert.Equal(t, "SELECT * FROM users WHERE id = ?", info.Query)
	assert.Len(t, info.Args, 1)
	assert.Equal(t, 1, info.Args[0])
	assert.Equal(t, 150*time.Millisecond, info.Duration)
}

func TestOptionsChaining(t *testing.T) {
	var hookCalled bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		hookCalled = true
	}

	observer := xmetrics.NoopObserver{}

	opts := defaultOptions()
	WithHealthTimeout(10 * time.Second)(opts)
	WithSlowQueryThreshold(200 * time.Millisecond)(opts)
	WithSlowQueryHook(hook)(opts)
	WithObserver(observer)(opts)

	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
	assert.Equal(t, 200*time.Millisecond, opts.SlowQueryThreshold)
	assert.NotNil(t, opts.SlowQueryHook)
	assert.Equal(t, observer, opts.Observer)

	// 验证 hook 可被调用
	opts.SlowQueryHook(context.Background(), SlowQueryInfo{})
	assert.True(t, hookCalled)
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
