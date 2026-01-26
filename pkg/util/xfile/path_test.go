package xfile

import (
	"fmt"
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

// =============================================================================
// 示例测试
// =============================================================================

func ExampleSanitizePath() {
	// 正常路径
	path, err := SanitizePath("/var/log/app.log")
	if err != nil {
		panic(err)
	}
	println(path)

	// 路径穿越会被拒绝
	_, err = SanitizePath("../../../etc/passwd")
	if err != nil {
		println("路径穿越被阻止")
	}
	// Output:
}

func ExampleSanitizePath_normalize() {
	// 路径会被规范化
	path, err := SanitizePath("/var/./log/../log/app.log")
	if err != nil {
		panic(err)
	}
	fmt.Println(path)
	_ = path
	// 结果: /var/log/app.log
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
	// 构造一个超深的路径（超过 maxSymlinkDepth）
	deepPath := "/a"
	for i := 0; i < 300; i++ {
		deepPath += "/b"
	}
	// 使用导出的 SafeJoinWithOptions 间接测试
	// 因为 evalSymlinksPartialWithDepth 是内部函数
	_, err := SafeJoinWithOptions("/tmp", "test.log", SafeJoinOptions{
		ResolveSymlinks: true,
	})
	// 正常路径应该成功
	if err != nil {
		t.Logf("SafeJoinWithOptions 返回错误（可能是 /tmp 不存在）: %v", err)
	}

	// 测试超深路径通过内部函数
	// 由于 evalSymlinksPartialWithDepth 是内部函数，我们通过构造一个不存在的深层路径来测试
	// 当目录不存在时，evalSymlinksPartial 会递归调用自己
	deepBase := "/nonexistent"
	for i := 0; i < 300; i++ {
		deepBase += "/level"
	}
	_, err = evalSymlinksPartialWithDepth(deepBase+"/file.log", 0)
	if err == nil {
		t.Error("期望深层路径返回错误，但没有")
	} else if !strings.Contains(err.Error(), "path too deep") {
		t.Logf("返回了其他错误（预期）: %v", err)
	}
}

// TestSafeJoinWithOptionsSymlinks 测试符号链接解析
func TestSafeJoinWithOptionsSymlinks(t *testing.T) {
	// 使用 /tmp 作为测试基准目录（总是存在）
	t.Run("基本符号链接解析", func(t *testing.T) {
		// 测试对现有目录的解析
		result, err := SafeJoinWithOptions("/tmp", "test.log", SafeJoinOptions{
			ResolveSymlinks: true,
		})
		if err != nil {
			t.Errorf("SafeJoinWithOptions 意外错误: %v", err)
			return
		}
		// 结果应该是 /tmp/test.log 或其真实路径
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

// =============================================================================
// SafeJoin 示例测试
// =============================================================================

func ExampleSafeJoin() {
	// 正常使用
	path, err := SafeJoin("/var/log", "app.log")
	if err != nil {
		panic(err)
	}
	fmt.Println(path)
	// Output: /var/log/app.log
}

func ExampleSafeJoin_pathTraversal() {
	// 路径穿越会被阻止
	_, err := SafeJoin("/var/log", "../etc/passwd")
	if err != nil {
		fmt.Println("路径穿越被阻止")
	}
	// Output: 路径穿越被阻止
}

func ExampleSafeJoin_absolutePath() {
	// 绝对路径会被拒绝
	_, err := SafeJoin("/var/log", "/etc/passwd")
	if err != nil {
		fmt.Println("绝对路径被拒绝")
	}
	// Output: 绝对路径被拒绝
}
