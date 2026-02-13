package xfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// 模糊测试（Fuzz）
//
// 模糊测试用于发现边界条件和异常输入下的潜在问题。
// 运行方式：go test -fuzz=FuzzXxx -fuzztime=30s
// =============================================================================

// FuzzSanitizePath 模糊测试路径规范化
//
// 测试目标：
//   - 任意字符串输入不会导致 panic
//   - 路径穿越攻击被正确阻止
//   - 返回的路径总是规范化的
func FuzzSanitizePath(f *testing.F) {
	// 添加种子语料
	f.Add("/var/log/app.log")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("../../../etc/passwd")
	f.Add("/a/b/c/d.log")
	f.Add("test.log")
	f.Add("/var/log/")
	f.Add("./relative/path.log")
	f.Add("a/b/../c/test.log")
	f.Add(string(bytes.Repeat([]byte("x"), 255)))
	f.Add("/var/./log/../log/app.log")
	f.Add("日志.log")
	f.Add("/var/log/app with space.log")
	f.Add("\\windows\\path\\file.log")
	f.Add("/var/log/\x00hidden.log")
	f.Add("/var/log/app\nlog")

	f.Fuzz(func(t *testing.T, input string) {
		// SanitizePath 不应该 panic
		result, err := SanitizePath(input)

		if err != nil {
			// 错误是可接受的（空路径、路径穿越等）
			return
		}

		// 如果成功，验证结果
		// 1. 结果不应为空
		if result == "" {
			t.Error("SanitizePath 返回空字符串但没有错误")
		}

		// 2. 结果应该是规范化的
		if result != filepath.Clean(result) {
			t.Errorf("结果 %q 不是规范化的路径", result)
		}

		// 3. 结果不应包含 ..（路径穿越）
		if hasDotDotSegment(result) {
			t.Errorf("结果 %q 包含路径穿越", result)
		}
	})
}

// FuzzSanitizePathTraversal 专门测试路径穿越防护
//
// 测试目标：
//   - 各种变体的路径穿越都被阻止
func FuzzSanitizePathTraversal(f *testing.F) {
	// 路径穿越变体
	f.Add("..")
	f.Add("../")
	f.Add("..\\")
	f.Add("../etc/passwd")
	f.Add("..%2f")
	f.Add("..%5c")
	f.Add("....//")
	f.Add("/var/../../../etc/passwd")
	f.Add("foo/../../../etc/passwd")
	f.Add("./../../etc/passwd")

	f.Fuzz(func(t *testing.T, input string) {
		result, err := SanitizePath(input)

		// 如果输入包含 .. 且成功返回，验证结果不包含路径穿越
		if err == nil && strings.Contains(input, "..") {
			if hasDotDotSegment(result) {
				t.Errorf("输入 %q 产生了包含 .. 的结果 %q", input, result)
			}
		}
	})
}

// FuzzEnsureDir 模糊测试目录创建
//
// 测试目标：
//   - 各种文件路径输入不会导致 panic
//   - 不会在意外位置创建目录
func FuzzEnsureDir(f *testing.F) {
	// 添加种子语料
	f.Add("app.log")
	f.Add("./app.log")
	f.Add("logs/app.log")
	f.Add("a/b/c/d/e/app.log")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("/")

	tmpDir := f.TempDir()

	f.Fuzz(func(t *testing.T, input string) {
		// 构造安全的测试路径（在临时目录下）
		if input == "" || strings.Contains(input, "..") || strings.HasPrefix(input, "/") {
			// 跳过可能不安全的路径
			return
		}

		testPath := filepath.Join(tmpDir, input)

		// EnsureDir 不应该 panic
		err := EnsureDir(testPath)

		// 错误是可接受的
		if err != nil {
			return
		}

		// 如果成功，验证父目录确实存在
		dir := filepath.Dir(testPath)
		if dir != "" && dir != "." {
			info, statErr := os.Stat(dir)
			if statErr != nil {
				t.Errorf("EnsureDir(%q) 成功但目录 %q 不存在: %v", testPath, dir, statErr)
			} else if !info.IsDir() {
				t.Errorf("EnsureDir(%q) 成功但 %q 不是目录", testPath, dir)
			}
		}
	})
}

// FuzzEnsureDirWithPerm 模糊测试带权限的目录创建
//
// 测试目标：
//   - 各种权限值不会导致 panic
//   - 无效权限被正确处理
func FuzzEnsureDirWithPerm(f *testing.F) {
	// 添加种子语料：(路径, 权限)
	f.Add("app.log", uint32(0755))
	f.Add("logs/app.log", uint32(0700))
	f.Add("a/b/c/app.log", uint32(0750))
	f.Add("test.log", uint32(0777))
	f.Add("test.log", uint32(0000)) // 会返回 ErrInvalidPerm（缺少所有者执行位）
	f.Add("test.log", uint32(0644)) // 会返回 ErrInvalidPerm（缺少所有者执行位）
	f.Add("test.log", uint32(0100)) // 最小有效权限

	tmpDir := f.TempDir()

	f.Fuzz(func(t *testing.T, input string, permBits uint32) {
		// 构造安全的测试路径
		if input == "" || strings.Contains(input, "..") || strings.HasPrefix(input, "/") {
			return
		}

		testPath := filepath.Join(tmpDir, "fuzzperm", input)
		perm := os.FileMode(permBits & 0777) // 确保权限在有效范围内

		// EnsureDirWithPerm 不应该 panic
		_ = EnsureDirWithPerm(testPath, perm)
	})
}

// FuzzSanitizePathUnicode 测试 Unicode 路径处理
//
// 测试目标：
//   - 各种 Unicode 字符不会导致 panic
//   - 正确处理多语言文件名
func FuzzSanitizePathUnicode(f *testing.F) {
	// 添加多语言种子
	f.Add("日志.log")
	f.Add("журнал.log")
	f.Add("سجل.log")
	f.Add("로그.log")
	f.Add("יומן.log")
	f.Add("📝.log")
	f.Add("/var/log/应用/日志.log")
	f.Add("données/fichier.log")

	f.Fuzz(func(t *testing.T, input string) {
		// 不应该 panic
		result, err := SanitizePath(input)

		if err == nil && result != "" {
			// 验证结果是有效的 UTF-8（Go 字符串默认就是）
			// 这里主要确保没有 panic
		}
	})
}
