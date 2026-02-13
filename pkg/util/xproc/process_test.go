package xproc

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetProcessName 重置进程名称缓存，仅供测试使用。
func resetProcessName() {
	processNameOnce = sync.Once{}
	processNameValue = ""
}

func TestProcessID(t *testing.T) {
	pid := ProcessID()
	assert.Greater(t, pid, 0)
	assert.Equal(t, os.Getpid(), pid)
}

func TestProcessName(t *testing.T) {
	resetProcessName()
	defer resetProcessName()

	name := ProcessName()
	assert.NotEmpty(t, name)
	// 应不含路径分隔符（filepath.Base 已剥离路径）
	assert.NotContains(t, name, string(os.PathSeparator))
}

func TestProcessName_Cached(t *testing.T) {
	origExec := osExecutable
	defer func() {
		osExecutable = origExec
		resetProcessName()
	}()

	resetProcessName()
	osExecutable = func() (string, error) {
		return "/opt/bin/cached-app", nil
	}

	// 首次调用触发解析
	name1 := ProcessName()
	assert.Equal(t, "cached-app", name1)

	// 修改 mock 后，缓存值不变
	osExecutable = func() (string, error) {
		return "/opt/bin/other-app", nil
	}
	name2 := ProcessName()
	assert.Equal(t, "cached-app", name2, "ProcessName 应返回缓存值")
}

func TestResolveProcessName_FallbackToArgs(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// 模拟 os.Executable 失败，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}

	name := resolveProcessName()
	assert.NotEmpty(t, name)
}

// 注意：此测试修改全局 os.Args，不可使用 t.Parallel()。
func TestResolveProcessName_EmptyArgs(t *testing.T) {
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
	assert.Equal(t, "", resolveProcessName())

	os.Args = []string{}
	assert.Equal(t, "", resolveProcessName())
}

func TestResolveProcessName_EmptyArg0(t *testing.T) {
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
	assert.Equal(t, "", resolveProcessName())
}

func TestResolveProcessName_PathStripping(t *testing.T) {
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
	require.Equal(t, "myapp", resolveProcessName())

	os.Args = []string{"./relative/path/app"}
	require.Equal(t, "app", resolveProcessName())
}

func TestResolveProcessName_OsExecutablePrimary(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// os.Executable 返回特定路径时，应优先使用
	osExecutable = func() (string, error) {
		return "/opt/bin/myservice", nil
	}

	assert.Equal(t, "myservice", resolveProcessName())
}

func TestResolveProcessName_SpecialPaths(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
		resetProcessName()
	}()

	specialPaths := []string{"/", ".", ".."}

	for _, path := range specialPaths {
		// os.Executable 返回特殊路径时应回退到 os.Args
		t.Run("exe_"+path+"_fallback", func(t *testing.T) {
			osExecutable = func() (string, error) {
				return path, nil
			}
			os.Args = []string{"/usr/bin/fallback"}
			assert.Equal(t, "fallback", resolveProcessName())
		})

		// os.Args[0] 为特殊路径时应返回空
		t.Run("args_"+path, func(t *testing.T) {
			osExecutable = func() (string, error) {
				return "", errors.New("not supported")
			}
			os.Args = []string{path}
			assert.Equal(t, "", resolveProcessName())
		})
	}
}

func TestResolveProcessName_OsExecutableEmpty(t *testing.T) {
	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	// os.Executable 返回空字符串时，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", nil
	}

	name := resolveProcessName()
	// 回退到 os.Args[0]
	assert.NotEmpty(t, name)
}
