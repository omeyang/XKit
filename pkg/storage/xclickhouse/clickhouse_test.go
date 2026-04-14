package xclickhouse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_NilConn(t *testing.T) {
	ch, err := New(nil)

	assert.Nil(t, ch)
	assert.ErrorIs(t, err, ErrNilClient)
}

// TestNew_TypedNilConn 验证 typed-nil driver.Conn 被拒绝。
// 设计决策: 仅用 client == nil 无法拦截 (*mockConn)(nil) 等 typed-nil，
// 后续 Ping/Stats 会解引用 nil 接收者 panic。
func TestNew_TypedNilConn(t *testing.T) {
	var typedNil *mockConn // typed-nil: 接口 type=*mockConn, value=nil
	ch, err := New(typedNil)

	assert.Nil(t, ch)
	assert.ErrorIs(t, err, ErrNilClient)
}

// TestNew_NilOption 验证 opts 切片含 nil 元素时不 panic。
func TestNew_NilOption(t *testing.T) {
	ch, err := New(newMockConn(), nil, WithHealthTimeout(1e9), nil)

	assert.NotNil(t, ch)
	assert.NoError(t, err)
	if ch != nil {
		assert.NoError(t, ch.Close())
	}
}

func TestClickHouseInterface(t *testing.T) {
	// 验证 clickhouseWrapper 实现了 ClickHouse 接口
	var _ ClickHouse = (*clickhouseWrapper)(nil)
}

func TestOptionsAreApplied(t *testing.T) {
	// 使用白盒测试验证选项被正确应用
	// 由于 New 需要真实连接，这里只测试选项应用逻辑
	opts := defaultOptions()

	WithHealthTimeout(10 * 1e9)(opts)       // 10 秒
	WithSlowQueryThreshold(100 * 1e6)(opts) // 100ms

	assert.Equal(t, int64(10*1e9), int64(opts.HealthTimeout))
	assert.Equal(t, int64(100*1e6), int64(opts.SlowQueryThreshold))
}
