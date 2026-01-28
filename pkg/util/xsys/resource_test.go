package xsys

import (
	"errors"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetFileLimit_ZeroValue(t *testing.T) {
	// 参数校验在所有平台上行为一致。
	err := SetFileLimit(0)
	require.ErrorIs(t, err, ErrInvalidFileLimit)
}

func TestSetFileLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		err := SetFileLimit(1024)
		require.ErrorIs(t, err, ErrUnsupportedPlatform)
		return
	}

	// Unix: 实际设置 RLIMIT_NOFILE，合理值应成功。
	err := SetFileLimit(1024)
	require.NoError(t, err)

	// 验证实际效果
	soft, _, err := GetFileLimit()
	require.NoError(t, err)
	assert.Equal(t, uint64(1024), soft)
}

func TestSetFileLimitHighValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}
	// 尝试设置极高值以覆盖 rlimit.Max < limit 分支。
	// 非特权进程通常会因 EPERM 失败。
	err := SetFileLimit(1 << 30)
	if err != nil {
		// 验证是预期的权限错误，而非代码 bug。
		assert.True(t,
			errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL),
			"unexpected error type: %v", err,
		)
	}
}

func TestGetFileLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		_, _, err := GetFileLimit()
		require.ErrorIs(t, err, ErrUnsupportedPlatform)
		return
	}

	soft, hard, err := GetFileLimit()
	require.NoError(t, err)
	assert.Greater(t, soft, uint64(0))
	assert.GreaterOrEqual(t, hard, soft)
}
