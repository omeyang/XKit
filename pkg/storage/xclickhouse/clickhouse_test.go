package xclickhouse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_NilConn(t *testing.T) {
	ch, err := New(nil)

	assert.Nil(t, ch)
	assert.ErrorIs(t, err, ErrNilConn)
}

func TestNew_WithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 需要真实的 ClickHouse 连接
}

func TestClickHouseInterface(t *testing.T) {
	// 验证 clickhouseWrapper 实现了 ClickHouse 接口
	var _ ClickHouse = (*clickhouseWrapper)(nil)
}

func TestOptionsApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 需要真实的 ClickHouse 连接
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
