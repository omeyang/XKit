//go:build unix

package xsys

import (
	"errors"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// 不可 t.Parallel()：修改进程 rlimit 全局状态，与其他测试互斥。
func TestSetFileLimit(t *testing.T) {
	// 保存原始 soft limit，测试结束后恢复。
	origSoft, hard, err := GetFileLimit()
	require.NoError(t, err)
	t.Cleanup(func() {
		if restoreErr := SetFileLimit(origSoft); restoreErr != nil {
			t.Errorf("restore rlimit: %v", restoreErr)
		}
	})

	// 使用相对于当前 hard limit 的值，避免在受限容器中因固定值假设而失败。
	target := hard / 2
	if target == 0 {
		target = 1
	}

	err = SetFileLimit(target)
	require.NoError(t, err)

	// 验证实际效果
	soft, _, err := GetFileLimit()
	require.NoError(t, err)
	assert.Equal(t, target, soft)
}

// 不可 t.Parallel()：修改进程 rlimit 全局状态，与其他测试互斥。
func TestSetFileLimit_HighValue(t *testing.T) {
	const highLimit = 1 << 30

	// 保存原始值，测试结束后恢复。
	origSoft, _, err := GetFileLimit()
	require.NoError(t, err)
	t.Cleanup(func() {
		if restoreErr := SetFileLimit(origSoft); restoreErr != nil {
			t.Errorf("restore rlimit: %v", restoreErr)
		}
	})

	// 尝试设置极高值以覆盖 rlimit.Max < limit 分支。
	// 非特权进程通常会因 EPERM 失败；特权进程应成功。
	err = SetFileLimit(highLimit)
	if err != nil {
		// 验证是预期的权限错误，而非代码 bug。
		assert.True(t,
			errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL),
			"unexpected error type: %v", err,
		)
		return
	}

	// 特权环境下设置成功，验证 soft limit 已生效。
	soft, _, err := GetFileLimit()
	require.NoError(t, err)
	assert.Equal(t, uint64(highLimit), soft)
}

// 不可 t.Parallel()：替换包级变量 getrlimit。
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

// 不可 t.Parallel()：替换包级变量 setrlimit。
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

// 不可 t.Parallel()：读取进程 rlimit 全局状态，与修改 rlimit 的测试互斥。
func TestGetFileLimit(t *testing.T) {
	soft, hard, err := GetFileLimit()
	require.NoError(t, err)
	assert.Greater(t, soft, uint64(0))
	assert.GreaterOrEqual(t, hard, soft)
}

// 不可 t.Parallel()：替换包级变量 getrlimit。
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

// 不可 t.Parallel()：修改进程 rlimit 全局状态，与其他测试互斥。
// TestSetFileLimit_Concurrent 验证 fileLimitMu 的并发保护。
// 配合 -race 运行，将 "并发安全" 从注释承诺提升为可执行契约。
func TestSetFileLimit_Concurrent(t *testing.T) {
	origSoft, hard, err := GetFileLimit()
	require.NoError(t, err)
	t.Cleanup(func() {
		if restoreErr := SetFileLimit(origSoft); restoreErr != nil {
			t.Errorf("restore rlimit: %v", restoreErr)
		}
	})

	// 使用不同的合法 target 值，均在 hard limit 范围内。
	targets := []uint64{hard / 4, hard / 3, hard / 2}
	for i, v := range targets {
		if v == 0 {
			targets[i] = 1
		}
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			if err := SetFileLimit(targets[idx%len(targets)]); err != nil {
				t.Errorf("concurrent SetFileLimit: %v", err)
			}
			if _, _, err := GetFileLimit(); err != nil {
				t.Errorf("concurrent GetFileLimit: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// 验证最终状态一致性。
	soft, finalHard, err := GetFileLimit()
	require.NoError(t, err)
	assert.LessOrEqual(t, soft, finalHard)
}
