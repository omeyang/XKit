package xproc

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 注意：本文件中的测试修改包级全局变量（osExecutable、os.Args、processNameOnce），
// 不可使用 t.Parallel()。每个测试通过 defer 恢复原始状态以避免污染后续测试。

func TestProcessID(t *testing.T) {
	pid := ProcessID()
	assert.Greater(t, pid, 0)
	assert.Equal(t, os.Getpid(), pid)
}

func TestProcessName(t *testing.T) {
	ResetProcessName()
	defer ResetProcessName()

	name := ProcessName()
	assert.NotEmpty(t, name)
	// 应不含路径分隔符（filepath.Base 已剥离路径）
	assert.NotContains(t, name, string(os.PathSeparator))
}

func TestProcessName_Cached(t *testing.T) {
	origExec := osExecutable
	defer func() {
		osExecutable = origExec
		ResetProcessName()
	}()

	ResetProcessName()
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
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	// 模拟 os.Executable 失败，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", errors.New("not supported")
	}
	os.Args = []string{"/usr/bin/test-app"}

	name := resolveProcessName()
	assert.Equal(t, "test-app", name)
}

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
	}()

	// 使用描述性标签替代原始路径值，避免 "/" 被 testing 包解释为子测试层级分隔符。
	specialPaths := []struct {
		label string
		path  string
	}{
		{"slash", "/"},
		{"dot", "."},
		{"dotdot", ".."},
	}

	for _, tc := range specialPaths {
		// os.Executable 返回特殊路径时应回退到 os.Args
		t.Run("exe_"+tc.label+"_fallback", func(t *testing.T) {
			osExecutable = func() (string, error) {
				return tc.path, nil
			}
			os.Args = []string{"/usr/bin/fallback"}
			assert.Equal(t, "fallback", resolveProcessName())
		})

		// os.Args[0] 为特殊路径时应返回空
		t.Run("args_"+tc.label, func(t *testing.T) {
			osExecutable = func() (string, error) {
				return "", errors.New("not supported")
			}
			os.Args = []string{tc.path}
			assert.Equal(t, "", resolveProcessName())
		})
	}
}

func TestResolveProcessName_OsExecutableEmpty(t *testing.T) {
	origExec := osExecutable
	origArgs := os.Args
	defer func() {
		osExecutable = origExec
		os.Args = origArgs
	}()

	// os.Executable 返回空字符串时，应回退到 os.Args[0]
	osExecutable = func() (string, error) {
		return "", nil
	}
	os.Args = []string{"/usr/bin/test-app"}

	name := resolveProcessName()
	assert.Equal(t, "test-app", name)
}
