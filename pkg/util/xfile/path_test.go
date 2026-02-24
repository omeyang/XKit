package xfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// SanitizePath 单元测试
// =============================================================================

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantErr   bool
		errSubstr string
	}{
		// 正常路径
		{
			name:  "绝对路径",
			input: "/var/log/app.log",
			want:  "/var/log/app.log",
		},
		{
			name:  "相对路径",
			input: "logs/app.log",
			want:  "logs/app.log",
		},
		{
			name:  "简单文件名",
			input: "app.log",
			want:  "app.log",
		},
		{
			name:  "带点的文件名",
			input: "app.2024.log",
			want:  "app.2024.log",
		},
		{
			name:  "文件名包含双点",
			input: "app..2024.log",
			want:  "app..2024.log",
		},
		{
			name:  "隐藏文件",
			input: ".gitignore",
			want:  ".gitignore",
		},
		{
			name:  "深层路径",
			input: "/a/b/c/d/e/f.log",
			want:  "/a/b/c/d/e/f.log",
		},

		// 路径规范化
		{
			name:  "带单点的路径",
			input: "/var/./log/./app.log",
			want:  "/var/log/app.log",
		},
		{
			name:  "重复斜杠",
			input: "/var//log///app.log",
			want:  "/var/log/app.log",
		},
		{
			name:  "相对路径内部 .. 被规范化",
			input: "a/../b/c.log",
			want:  "b/c.log",
		},
		{
			name:  "多层相对路径内部 .. 被规范化",
			input: "a/b/../c/d.log",
			want:  "a/c/d.log",
		},

		// 错误情况
		{
			name:      "空路径",
			input:     "",
			wantErr:   true,
			errSubstr: "filename is required",
		},
		{
			name:      "目录路径（尾部斜杠）",
			input:     "/var/log/",
			wantErr:   true,
			errSubstr: "path is a directory",
		},
		{
			name:      "路径穿越 - 相对路径",
			input:     "../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "路径穿越 - 多层相对",
			input:     "../../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:    "绝对路径带双点 - 被规范化",
			input:   "/var/log/../../../etc/passwd",
			want:    "/etc/passwd", // filepath.Clean 解析为有效绝对路径
			wantErr: false,
		},
		{
			name:    "绝对路径带双点 - 中间",
			input:   "/var/../../../etc/passwd",
			want:    "/etc/passwd", // filepath.Clean 解析为有效绝对路径
			wantErr: false,
		},
		{
			name:      "纯点",
			input:     ".",
			wantErr:   true,
			errSubstr: "no file name specified",
		},
		{
			name:      "路径含空字节",
			input:     "/var/log/app\x00.log",
			wantErr:   true,
			errSubstr: "null byte",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizePath(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SanitizePath(%q) 期望错误，但没有返回错误", tt.input)
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("SanitizePath(%q) 错误 = %q, 期望包含 %q", tt.input, err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("SanitizePath(%q) 意外错误: %v", tt.input, err)
				return
			}

			if got != tt.want {
				t.Errorf("SanitizePath(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSanitizePathEdgeCases 测试边界情况
func TestSanitizePathEdgeCases(t *testing.T) {
	// 测试各种特殊字符
	specialCases := []struct {
		name  string
		input string
	}{
		{"带空格", "/var/log/my app.log"},
		{"带中文", "/var/log/日志.log"},
		{"带特殊字符", "/var/log/app-v1.0_test.log"},
		{"带括号", "/var/log/app(1).log"},
	}

	for _, tc := range specialCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SanitizePath(tc.input)
			if err != nil {
				t.Errorf("SanitizePath(%q) 意外错误: %v", tc.input, err)
				return
			}
			// 验证返回的路径是规范化的
			if result != filepath.Clean(tc.input) {
				t.Errorf("SanitizePath(%q) = %q, 期望 %q", tc.input, result, filepath.Clean(tc.input))
			}
		})
	}
}

// TestSanitizePathCrossPlatform 测试跨平台路径处理
func TestSanitizePathCrossPlatform(t *testing.T) {
	// 测试当前平台的路径分隔符行为
	t.Run("使用平台路径分隔符", func(t *testing.T) {
		input := filepath.Join("var", "log", "app.log")
		result, err := SanitizePath(input)
		if err != nil {
			t.Errorf("SanitizePath(%q) 意外错误: %v", input, err)
			return
		}
		expected := filepath.Clean(input)
		if result != expected {
			t.Errorf("SanitizePath(%q) = %q, 期望 %q", input, result, expected)
		}
	})
}

// TestSanitizePathWindowsInputs 测试 Windows 风格路径输入
func TestSanitizePathWindowsInputs(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		// SanitizePath 不拒绝 Windows 格式路径（那是 SafeJoin 的职责）
		{
			name:    "Windows 驱动器绝对路径 - 通过",
			input:   `C:\Windows\System32\cmd.exe`,
			wantErr: false,
		},
		{
			name:    "Windows UNC 路径 - 通过",
			input:   `\\server\share\file.txt`,
			wantErr: false,
		},
		{
			name:    "Windows 驱动器路径正斜杠",
			input:   "C:/Users/test/file.txt",
			wantErr: false,
		},
		{
			name:      "Windows 尾部反斜杠 - 拒绝",
			input:     `C:\Windows\`,
			wantErr:   true,
			errSubstr: "path is a directory", // 尾部 \ 被视为目录标记
		},
		{
			name:    "Windows 驱动器相对路径",
			input:   "C:foo.log",
			wantErr: false,
		},
		{
			name:      "Windows 路径穿越",
			input:     `subdir\..\..\..\etc\passwd`,
			wantErr:   true,
			errSubstr: "path traversal", // hasDotDotSegment 检测 ".." 段
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SanitizePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SanitizePath(%q) 期望错误，但没有返回错误", tt.input)
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("SanitizePath(%q) 错误 = %q, 期望包含 %q", tt.input, err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Errorf("SanitizePath(%q) 意外错误: %v", tt.input, err)
			}
		})
	}
}

// =============================================================================
// SafeJoin 单元测试
// =============================================================================

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		path      string
		want      string
		wantErr   bool
		errSubstr string
	}{
		// 正常情况
		{
			name: "简单文件名",
			base: "/var/log",
			path: "app.log",
			want: "/var/log/app.log",
		},
		{
			name: "子目录文件",
			base: "/var/log",
			path: "myapp/app.log",
			want: "/var/log/myapp/app.log",
		},
		{
			name: "带点的文件名",
			base: "/var/log",
			path: "app.2024.01.01.log",
			want: "/var/log/app.2024.01.01.log",
		},
		{
			name: "文件名包含双点",
			base: "/var/log",
			path: "app..2024.log",
			want: "/var/log/app..2024.log",
		},
		{
			name: "隐藏文件",
			base: "/var/log",
			path: ".gitignore",
			want: "/var/log/.gitignore",
		},
		{
			name: "当前目录表示",
			base: "/var/log",
			path: "./app.log",
			want: "/var/log/app.log",
		},
		// 以双点开头的合法文件名（不应被误判为路径穿越）
		{
			name: "双点开头的文件名",
			base: "/var/log",
			path: "..config",
			want: "/var/log/..config",
		},
		{
			name: "双点开头的隐藏文件",
			base: "/var/log",
			path: "...hidden",
			want: "/var/log/...hidden",
		},
		{
			name: "子目录下双点开头的文件",
			base: "/var/log",
			path: "subdir/..settings",
			want: "/var/log/subdir/..settings",
		},

		// 安全阻止 - 路径穿越
		{
			name:      "路径穿越 - 简单",
			base:      "/var/log",
			path:      "../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "路径穿越 - 多层",
			base:      "/var/log",
			path:      "../../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "路径穿越 - 中间目录",
			base:      "/var/log",
			path:      "subdir/../../../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "路径穿越 - 深层",
			base:      "/var/log",
			path:      "a/b/c/../../../../etc/passwd",
			wantErr:   true,
			errSubstr: "path traversal",
		},

		// 安全阻止 - 绝对路径
		{
			name:      "绝对路径拒绝",
			base:      "/var/log",
			path:      "/etc/passwd",
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},

		// 安全阻止 - Windows 绝对路径（跨平台防护）
		{
			name:      "Windows 驱动器号",
			base:      "/var/log",
			path:      `C:\Windows\System32\config`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 驱动器号小写",
			base:      "/var/log",
			path:      `d:\data\file.txt`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 驱动器号正斜杠",
			base:      "/var/log",
			path:      "C:/Users/test/file.txt",
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows UNC 路径",
			base:      "/var/log",
			path:      `\\server\share\file.txt`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 根路径（单反斜杠）",
			base:      "/var/log",
			path:      `\Windows\System32\config`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 根路径（单反斜杠简单）",
			base:      "/var/log",
			path:      `\foo`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 驱动器相对路径",
			base:      "/var/log",
			path:      "C:foo",
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "Windows 驱动器相对路径穿越",
			base:      "/var/log",
			path:      `C:..\bar`,
			wantErr:   true,
			errSubstr: "absolute path not allowed",
		},
		{
			name:      "路径含空字节",
			base:      "/var/log",
			path:      "app\x00.log",
			wantErr:   true,
			errSubstr: "null byte",
		},

		// 参数验证
		{
			name:      "空 base",
			base:      "",
			path:      "app.log",
			wantErr:   true,
			errSubstr: "base directory is required",
		},
		{
			name:      "空 path",
			base:      "/var/log",
			path:      "",
			wantErr:   true,
			errSubstr: "path is required",
		},
		{
			name:      "base 非绝对路径",
			base:      "var/log",
			path:      "app.log",
			wantErr:   true,
			errSubstr: "base must be an absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(tt.base, tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SafeJoin(%q, %q) 期望错误，但返回 %q", tt.base, tt.path, got)
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("SafeJoin(%q, %q) 错误 = %q, 期望包含 %q", tt.base, tt.path, err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("SafeJoin(%q, %q) 意外错误: %v", tt.base, tt.path, err)
				return
			}

			if got != tt.want {
				t.Errorf("SafeJoin(%q, %q) = %q, 期望 %q", tt.base, tt.path, got, tt.want)
			}
		})
	}
}

// TestSafeJoinVsSanitizePath 对比 SafeJoin 和 SanitizePath 的行为差异
// TestJoinAndVerifyRelError 测试 joinAndVerify 中 filepath.Rel 失败的防御性分支。
// filepath.Rel 对两个已清理的绝对路径在当前标准库实现中不会失败，
// 此测试通过注入模拟实现覆盖该防御性代码路径。
func TestJoinAndVerifyRelError(t *testing.T) {
	original := filepathRelFn
	t.Cleanup(func() { filepathRelFn = original })

	filepathRelFn = func(_, _ string) (string, error) {
		return "", errors.New("simulated Rel failure")
	}

	_, err := SafeJoin("/var/log", "app.log")
	if err == nil {
		t.Fatal("期望 filepathRelFn 失败时 SafeJoin 返回错误")
	}
	if !errors.Is(err, ErrPathEscaped) {
		t.Errorf("期望 ErrPathEscaped，但得到: %v", err)
	}
}

func TestSafeJoinVsSanitizePath(t *testing.T) {
	t.Run("SanitizePath允许绝对路径穿越而SafeJoin不允许", func(t *testing.T) {
		// SanitizePath 允许绝对路径穿越（已知的设计局限）
		result, err := SanitizePath("/var/log/../../../etc/passwd")
		if err != nil {
			t.Errorf("SanitizePath 预期不报错，但得到: %v", err)
		}
		if result != "/etc/passwd" {
			t.Errorf("SanitizePath 预期返回 /etc/passwd，但得到: %s", result)
		}

		// SafeJoin 正确阻止这种穿越
		_, err = SafeJoin("/var/log", "../../../etc/passwd")
		if err == nil {
			t.Error("SafeJoin 应该阻止路径穿越，但没有报错")
		}
	})
}

// TestEvalSymlinksPartialDeepPath 测试深层路径不会栈溢出
func TestEvalSymlinksPartialDeepPath(t *testing.T) {
	// 使用导出的 SafeJoinWithOptions 间接测试正常路径
	tmpDir := t.TempDir()
	_, err := SafeJoinWithOptions(tmpDir, "test.log", SafeJoinOptions{
		ResolveSymlinks: true,
	})
	if err != nil {
		t.Errorf("SafeJoinWithOptions 意外错误: %v", err)
	}

	// 测试超深路径（超过 maxSymlinkDepth=255 层）
	// 当目录不存在时，evalSymlinksPartial 会迭代向上查找可解析祖先
	deepBase := "/nonexistent"
	for i := 0; i < 300; i++ {
		deepBase += "/level"
	}
	_, err = evalSymlinksPartial(deepBase + "/file.log")
	if err == nil {
		t.Fatal("期望深层路径返回错误，但没有")
	}
	if !errors.Is(err, ErrPathTooDeep) {
		t.Errorf("期望 ErrPathTooDeep 错误，但得到: %v", err)
	}
}

// TestSafeJoinWithOptionsSymlinks 测试符号链接解析
func TestSafeJoinWithOptionsSymlinks(t *testing.T) {
	t.Run("基本符号链接解析", func(t *testing.T) {
		tmpDir := t.TempDir()
		result, err := SafeJoinWithOptions(tmpDir, "test.log", SafeJoinOptions{
			ResolveSymlinks: true,
		})
		if err != nil {
			t.Errorf("SafeJoinWithOptions 意外错误: %v", err)
			return
		}
		if !strings.HasSuffix(result, "test.log") {
			t.Errorf("期望路径以 test.log 结尾，但得到: %s", result)
		}
	})

	t.Run("不存在的base目录", func(t *testing.T) {
		_, err := SafeJoinWithOptions("/nonexistent/path", "test.log", SafeJoinOptions{
			ResolveSymlinks: true,
		})
		if err == nil {
			t.Error("期望对不存在的目录报错")
		}
	})
}

// TestSafeJoinWithOptionsErrors 测试 SafeJoinWithOptions 各错误路径
func TestSafeJoinWithOptionsErrors(t *testing.T) {
	t.Run("validateBase 错误", func(t *testing.T) {
		_, err := SafeJoinWithOptions("", "app.log", SafeJoinOptions{})
		if err == nil {
			t.Error("期望错误，但没有返回错误")
		}
	})

	t.Run("validatePath 错误", func(t *testing.T) {
		_, err := SafeJoinWithOptions("/var/log", "", SafeJoinOptions{})
		if err == nil {
			t.Error("期望错误，但没有返回错误")
		}
	})

	t.Run("resolveSymlinks base 不存在", func(t *testing.T) {
		_, err := SafeJoinWithOptions("/nonexistent_base_dir_xfile_test", "app.log", SafeJoinOptions{
			ResolveSymlinks: true,
		})
		if err == nil {
			t.Error("期望错误，但没有返回错误")
		}
		if !errors.Is(err, ErrSymlinkResolution) {
			t.Errorf("期望 ErrSymlinkResolution，但得到: %v", err)
		}
	})
}

// TestSafeJoinSymlinkEscape 测试符号链接逃逸检测
func TestSafeJoinSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()

	// 在 tmpDir 内创建子目录作为 base
	baseDir := filepath.Join(tmpDir, "base")
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		t.Fatalf("创建 base 目录失败: %v", err)
	}

	// 在 base 内创建指向 base 外部的符号链接
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0750); err != nil {
		t.Fatalf("创建 outside 目录失败: %v", err)
	}
	secretFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0600); err != nil {
		t.Fatalf("创建 secret 文件失败: %v", err)
	}

	// 创建符号链接: base/evil -> outside
	evilLink := filepath.Join(baseDir, "evil")
	if err := os.Symlink(outsideDir, evilLink); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	t.Run("无符号链接解析时允许通过", func(t *testing.T) {
		// 不启用 ResolveSymlinks 时，路径检查不感知符号链接
		result, err := SafeJoinWithOptions(baseDir, "evil/secret.txt", SafeJoinOptions{
			ResolveSymlinks: false,
		})
		if err != nil {
			t.Errorf("期望不启用符号链接解析时通过，但得到错误: %v", err)
		}
		expected := filepath.Join(baseDir, "evil/secret.txt")
		if result != expected {
			t.Errorf("结果 = %q, 期望 %q", result, expected)
		}
	})

	t.Run("启用符号链接解析时检测逃逸", func(t *testing.T) {
		// 启用 ResolveSymlinks 时，应检测到路径逃逸
		_, err := SafeJoinWithOptions(baseDir, "evil/secret.txt", SafeJoinOptions{
			ResolveSymlinks: true,
		})
		if err == nil {
			t.Error("期望检测到符号链接逃逸，但没有报错")
		}
		if !errors.Is(err, ErrPathEscaped) {
			t.Errorf("期望 ErrPathEscaped 错误，但得到: %v", err)
		}
	})
}

// =============================================================================
// 错误类型测试（errors.Is 语义）
// =============================================================================

func TestSanitizePathErrorTypes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"空路径", "", ErrEmptyPath},
		{"目录路径", "/var/log/", ErrInvalidPath},
		{"路径穿越", "../etc/passwd", ErrPathTraversal},
		{"纯点", ".", ErrInvalidPath},
		{"空字节", "/var/log/\x00app.log", ErrNullByte},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SanitizePath(tt.input)
			if err == nil {
				t.Fatalf("SanitizePath(%q) 期望错误，但没有返回错误", tt.input)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("SanitizePath(%q) 错误 = %v, errors.Is(%v) = false", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSafeJoinErrorTypes(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		wantErr error
	}{
		{"空 base", "", "app.log", ErrEmptyPath},
		{"空 path", "/var/log", "", ErrEmptyPath},
		{"base 非绝对路径", "var/log", "app.log", ErrInvalidPath},
		{"绝对路径拒绝", "/var/log", "/etc/passwd", ErrInvalidPath},
		{"Windows 驱动器号", "/var/log", `C:\Windows\cmd.exe`, ErrInvalidPath},
		{"Windows UNC 路径", "/var/log", `\\server\share`, ErrInvalidPath},
		{"Windows 根路径", "/var/log", `\Windows\System32`, ErrInvalidPath},
		{"Windows 驱动器相对路径", "/var/log", "C:foo", ErrInvalidPath},
		{"空字节", "/var/log", "app\x00.log", ErrNullByte},
		{"base 含空字节", "/var/\x00log", "app.log", ErrNullByte},
		{"路径穿越", "/var/log", "../etc/passwd", ErrPathTraversal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoin(tt.base, tt.path)
			if err == nil {
				t.Fatalf("SafeJoin(%q, %q) 期望错误，但没有返回错误", tt.base, tt.path)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("SafeJoin(%q, %q) 错误 = %v, errors.Is(%v) = false", tt.base, tt.path, err, tt.wantErr)
			}
		})
	}
}
