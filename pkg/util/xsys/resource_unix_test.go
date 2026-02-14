//go:build unix

package xsys

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSetFileLimit(t *testing.T) {
	// 保存原始 soft limit，测试结束后恢复。
	origSoft, _, err := GetFileLimit()
	require.NoError(t, err)
	defer func() {
		// best-effort 恢复；测试环境中忽略恢复失败。
		if restoreErr := SetFileLimit(origSoft); restoreErr != nil {
			t.Logf("restore rlimit: %v", restoreErr)
		}
	}()

	// Unix: 实际设置 RLIMIT_NOFILE，合理值应成功。
	err = SetFileLimit(1024)
	require.NoError(t, err)

	// 验证实际效果
	soft, _, err := GetFileLimit()
	require.NoError(t, err)
	assert.Equal(t, uint64(1024), soft)
}

func TestSetFileLimit_HighValue(t *testing.T) {
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

func TestSetFileLimit_GetrlimitError(t *testing.T) {
	origGet := getrlimit
	defer func() { getrlimit = origGet }()

	mockErr := errors.New("mock getrlimit error")
	getrlimit = func(_ int, _ *unix.Rlimit) error {
		return mockErr
	}

	err := SetFileLimit(1024)
	require.ErrorIs(t, err, mockErr)
}

func TestSetFileLimit_SetrlimitError(t *testing.T) {
	origSet := setrlimit
	defer func() { setrlimit = origSet }()

	mockErr := errors.New("mock setrlimit error")
	setrlimit = func(_ int, _ *unix.Rlimit) error {
		return mockErr
	}

	err := SetFileLimit(1024)
	require.ErrorIs(t, err, mockErr)
}

func TestGetFileLimit(t *testing.T) {
	soft, hard, err := GetFileLimit()
	require.NoError(t, err)
	assert.Greater(t, soft, uint64(0))
	assert.GreaterOrEqual(t, hard, soft)
}

func TestGetFileLimit_GetrlimitError(t *testing.T) {
	origGet := getrlimit
	defer func() { getrlimit = origGet }()

	mockErr := errors.New("mock getrlimit error")
	getrlimit = func(_ int, _ *unix.Rlimit) error {
		return mockErr
	}

	soft, hard, err := GetFileLimit()
	require.ErrorIs(t, err, mockErr)
	assert.Equal(t, uint64(0), soft)
	assert.Equal(t, uint64(0), hard)
}
