package xproc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessID(t *testing.T) {
	pid := ProcessID()
	assert.Greater(t, pid, 0)
	assert.Equal(t, os.Getpid(), pid)
}

func TestProcessName(t *testing.T) {
	name := ProcessName()
	assert.NotEmpty(t, name)
	// 应不含路径分隔符（filepath.Base 已剥离路径）
	assert.NotContains(t, name, string(os.PathSeparator))
}

// 注意：此测试修改全局 os.Args，不可使用 t.Parallel()。
func TestProcessNameEmptyArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = nil
	assert.Equal(t, "", ProcessName())

	os.Args = []string{}
	assert.Equal(t, "", ProcessName())
}

func TestProcessNameEmptyArg0(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// os.Args[0] 为空字符串时应返回 ""，而非 filepath.Base("") 的 "."
	os.Args = []string{""}
	assert.Equal(t, "", ProcessName())
}

func TestProcessNamePathStripping(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"/usr/bin/myapp"}
	require.Equal(t, "myapp", ProcessName())

	os.Args = []string{"./relative/path/app"}
	require.Equal(t, "app", ProcessName())
}
