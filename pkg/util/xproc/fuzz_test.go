package xproc

import (
	"errors"
	"os"
	"testing"
)

func FuzzProcessName_ArgsFallback(f *testing.F) {
	// 添加种子语料：各种 os.Args[0] 格式
	f.Add("")
	f.Add("myapp")
	f.Add("/usr/bin/myapp")
	f.Add("./relative/path/app")
	f.Add("../parent/app")
	f.Add("/")
	f.Add(".")
	f.Add("..")
	f.Add("app with spaces")
	f.Add("/path/with spaces/app")
	f.Add("中文进程名")

	f.Fuzz(func(t *testing.T, arg0 string) {
		origExec := osExecutable
		origArgs := os.Args
		defer func() {
			osExecutable = origExec
			os.Args = origArgs
		}()

		// 强制走 os.Args[0] 回退路径
		osExecutable = func() (string, error) {
			return "", errors.New("not supported")
		}

		os.Args = []string{arg0}

		name := resolveProcessName()

		// 空 arg0 应返回空字符串
		if arg0 == "" {
			if name != "" {
				t.Errorf("resolveProcessName() = %q for empty arg0, want empty", name)
			}
			return
		}

		// 非空 arg0 不应 panic，结果长度不应超过输入
		if len(name) > len(arg0) {
			t.Errorf("resolveProcessName() = %q longer than input %q", name, arg0)
		}
	})
}

func FuzzProcessName_Executable(f *testing.F) {
	// 种子语料：各种 os.Executable 返回路径格式
	f.Add("")
	f.Add("myapp")
	f.Add("/usr/bin/myapp")
	f.Add("/opt/长路径/服务")
	f.Add("/")
	f.Add(".")
	f.Add("..")
	f.Add("/path/with spaces/app")

	f.Fuzz(func(t *testing.T, exePath string) {
		origExec := osExecutable
		origArgs := os.Args
		defer func() {
			osExecutable = origExec
			os.Args = origArgs
		}()

		osExecutable = func() (string, error) {
			return exePath, nil
		}
		// 清空 os.Args 以隔离 os.Executable 路径的影响
		os.Args = nil

		name := resolveProcessName()

		// 结果是 baseName(exePath)，长度不应超过输入
		if len(name) > len(exePath) {
			t.Errorf("resolveProcessName() = %q longer than input %q", name, exePath)
		}
	})
}
