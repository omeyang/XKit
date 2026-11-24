package xfile

import (
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

type safeJoinCase struct {
	name      string
	base      string
	path      string
	want      string
	wantErr   bool
	errSubstr string
}

func relativePathCases() []safeJoinCase {
	return []safeJoinCase{
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
	}
}

func rejectedPathCases() []safeJoinCase {
	return []safeJoinCase{
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
	}
}

func windowsAndInputCases() []safeJoinCase {
	return []safeJoinCase{
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
}
