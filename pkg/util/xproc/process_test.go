package xproc

import (
	"errors"
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

func TestProcessName_FallbackToArgs(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// 模拟 os.Executable 失败，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}

	name := ProcessName()
	assert.NotEmpty(t, name)
}

// 注意：此测试修改全局 os.Args，不可使用 t.Parallel()。
func TestProcessNameEmptyArgs(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	// 模拟 os.Executable 失败
	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}

	os.Args = nil
	assert.Equal(t, "", ProcessName())

	os.Args = []string{}
	assert.Equal(t, "", ProcessName())
}

func TestProcessNameEmptyArg0(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}

	// os.Args[0] 为空字符串时应返回 ""，而非 filepath.Base("") 的 "."
	os.Args = []string{""}
	assert.Equal(t, "", ProcessName())
}

func TestProcessNamePathStripping(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}

	os.Args = []string{"/usr/bin/myapp"}
	require.Equal(t, "myapp", ProcessName())

	os.Args = []string{"./relative/path/app"}
	require.Equal(t, "app", ProcessName())
}

func TestProcessName_OsExecutablePrimary(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// os.Executable 返回特定路径时，应优先使用
	osExecutable = func() (string, error) {
		return "/opt/bin/myservice", nil
	}

	assert.Equal(t, "myservice", ProcessName())
}

func TestProcessName_SpecialPaths(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	specialPaths := []string{"/", ".", ".."}

	for _, path := range specialPaths {
		// os.Executable 返回特殊路径时应回退到 os.Args
		t.Run("exe_"+path+"_fallback", func(t *testing.T) {
			osExecutable = func() (string, error) {
				return path, nil
			}
			os.Args = []string{"/usr/bin/fallback"}
			assert.Equal(t, "fallback", ProcessName())
		})

		// os.Args[0] 为特殊路径时应返回空
		t.Run("args_"+path, func(t *testing.T) {
			osExecutable = func() (string, error) {
				return "", errors.New("not supported")
			}
			os.Args = []string{path}
			assert.Equal(t, "", ProcessName())
		})
	}
}

func TestProcessName_OsExecutableEmpty(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// os.Executable 返回空字符串时，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", nil
	}

	name := ProcessName()
	// 回退到 os.Args[0]
	assert.NotEmpty(t, name)
}
