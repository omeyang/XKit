package xfile

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// EnsureDir 单元测试
// =============================================================================

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "创建单层目录",
			filename: filepath.Join(tmpDir, "newdir", "app.log"),
			wantErr:  false,
		},
		{
			name:     "创建多层目录",
			filename: filepath.Join(tmpDir, "a", "b", "c", "d", "app.log"),
			wantErr:  false,
		},
		{
			name:     "目录已存在",
			filename: filepath.Join(tmpDir, "app.log"),
			wantErr:  false,
		},
		{
			name:     "当前目录文件",
			filename: "app.log",
			wantErr:  false,
		},
		{
			name:     "相对路径单点",
			filename: "./app.log",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsureDir(tt.filename)

			if tt.wantErr {
				if err == nil {
					t.Error("EnsureDir() 期望错误，但没有返回错误")
				}
				return
			}

			if err != nil {
				t.Errorf("EnsureDir() 意外错误: %v", err)
				return
			}

			// 验证目录确实被创建
			dir := filepath.Dir(tt.filename)
			if dir != "" && dir != "." {
				info, err := os.Stat(dir)
				if err != nil {
					t.Errorf("目录 %q 未被创建: %v", dir, err)
					return
				}
				if !info.IsDir() {
					t.Errorf("%q 不是目录", dir)
				}
			}
		})
	}
}

func TestEnsureDirPermission(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "permtest", "app.log")

	err := EnsureDir(testDir)
	if err != nil {
		t.Fatalf("EnsureDir() 错误: %v", err)
	}

	dir := filepath.Dir(testDir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("无法获取目录信息: %v", err)
	}

	// 检查权限（注意：实际权限可能受 umask 影响）
	perm := info.Mode().Perm()
	// 验证所有者有读写执行权限
	if perm&0700 != 0700 {
		t.Errorf("目录权限 %o 不符合预期，所有者应有 rwx 权限", perm)
	}
}

// =============================================================================
// EnsureDirWithPerm 单元测试
// =============================================================================

func TestEnsureDirWithPerm(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		filename string
		perm     os.FileMode
		wantErr  bool
	}{
		{
			name:     "权限 0755",
			filename: filepath.Join(tmpDir, "perm755", "app.log"),
			perm:     0755,
			wantErr:  false,
		},
		{
			name:     "权限 0700",
			filename: filepath.Join(tmpDir, "perm700", "app.log"),
			perm:     0700,
			wantErr:  false,
		},
		{
			name:     "权限 0750",
			filename: filepath.Join(tmpDir, "perm750", "app.log"),
			perm:     0750,
			wantErr:  false,
		},
		{
			name:     "多层目录",
			filename: filepath.Join(tmpDir, "multi", "level", "dir", "app.log"),
			perm:     0755,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsureDirWithPerm(tt.filename, tt.perm)

			if tt.wantErr {
				if err == nil {
					t.Error("EnsureDirWithPerm() 期望错误，但没有返回错误")
				}
				return
			}

			if err != nil {
				t.Errorf("EnsureDirWithPerm() 意外错误: %v", err)
				return
			}

			// 验证目录存在
			dir := filepath.Dir(tt.filename)
			info, err := os.Stat(dir)
			if err != nil {
				t.Errorf("目录 %q 未被创建: %v", dir, err)
				return
			}
			if !info.IsDir() {
				t.Errorf("%q 不是目录", dir)
			}
		})
	}
}

func TestEnsureDirWithPermEdgeCases(t *testing.T) {
	t.Run("空目录路径", func(t *testing.T) {
		// filepath.Dir("app.log") 返回 "."
		err := EnsureDirWithPerm("app.log", 0755)
		if err != nil {
			t.Errorf("EnsureDirWithPerm() 意外错误: %v", err)
		}
	})

	t.Run("点路径", func(t *testing.T) {
		// filepath.Dir("./app.log") 返回 "."
		err := EnsureDirWithPerm("./app.log", 0755)
		if err != nil {
			t.Errorf("EnsureDirWithPerm() 意外错误: %v", err)
		}
	})

	t.Run("已存在的目录不报错", func(t *testing.T) {
		tmpDir := t.TempDir()
		filename := filepath.Join(tmpDir, "app.log")

		// 第一次调用
		err := EnsureDirWithPerm(filename, 0755)
		if err != nil {
			t.Errorf("第一次调用错误: %v", err)
		}

		// 第二次调用（目录已存在）
		err = EnsureDirWithPerm(filename, 0755)
		if err != nil {
			t.Errorf("第二次调用错误: %v", err)
		}
	})
}

// =============================================================================
// DefaultDirPerm 常量测试
// =============================================================================

func TestDefaultDirPerm(t *testing.T) {
	// 验证默认权限值
	if DefaultDirPerm != 0750 {
		t.Errorf("DefaultDirPerm = %o, 期望 0750", DefaultDirPerm)
	}
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleEnsureDir() {
	// 确保日志文件的父目录存在
	err := EnsureDir("/var/log/myapp/app.log")
	if err != nil {
		// 处理错误
		_ = err
	}
	// 现在可以安全地创建文件 /var/log/myapp/app.log
}

func ExampleEnsureDirWithPerm() {
	// 使用自定义权限创建目录
	err := EnsureDirWithPerm("/var/log/myapp/app.log", 0700)
	if err != nil {
		// 处理错误
		_ = err
	}
	// 目录 /var/log/myapp 将以 0700 权限创建
}
