//go:build freebsd || dragonfly

package xsys

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rlimit 字段为 int64 的平台：验证边界与溢出。
func TestRlimConversion_Int64Platform(t *testing.T) {
	t.Parallel()

	// 正常范围。
	got, err := rlimFromUint64(1024)
	require.NoError(t, err)
	assert.EqualValues(t, int64(1024), got)
	assert.Equal(t, uint64(1024), rlimToUint64(got))

	// 边界值 MaxInt64。
	got, err = rlimFromUint64(math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, uint64(math.MaxInt64), rlimToUint64(got))

	// 溢出。
	_, err = rlimFromUint64(math.MaxInt64 + 1)
	require.ErrorIs(t, err, ErrFileLimitOverflow)

	// 负值（理论上不会出现）安全截断为 0。
	assert.Equal(t, uint64(0), rlimToUint64(-1))
}

// 验证 SetFileLimit 在溢出时返回 ErrFileLimitOverflow。
// 不可 t.Parallel()：与其他修改 rlimit 的测试互斥。
func TestSetFileLimit_Overflow(t *testing.T) {
	err := SetFileLimit(math.MaxInt64 + 1)
	require.ErrorIs(t, err, ErrFileLimitOverflow)
}
