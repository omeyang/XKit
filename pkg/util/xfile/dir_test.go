package xfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestEnsureDirWithPermInvalidPerm(t *testing.T) {
	tests := []struct {
		name    string
		perm    os.FileMode
		wantErr bool
	}{
		{"零权限", 0o000, true},
		{"缺少执行位 0644", 0o644, true},
		{"缺少执行位 0060", 0o060, true},
		{"最小有效权限 0100", 0o100, false},
		{"标准权限 0700", 0o700, false},
		{"标准权限 0750", 0o750, false},
		{"标准权限 0755", 0o755, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filename := filepath.Join(tmpDir, "permcheck", "app.log")
			err := EnsureDirWithPerm(filename, tt.perm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("EnsureDirWithPerm(perm=%04o) 期望错误，但没有返回错误", tt.perm)
				} else if !errors.Is(err, ErrInvalidPerm) {
					t.Errorf("EnsureDirWithPerm(perm=%04o) 错误 = %v, errors.Is(ErrInvalidPerm) = false", tt.perm, err)
				}
			} else if err != nil {
				t.Errorf("EnsureDirWithPerm(perm=%04o) 意外错误: %v", tt.perm, err)
			}
		})
	}
}

func TestEnsureDirWithPermEmptyFilename(t *testing.T) {
	err := EnsureDirWithPerm("", 0755)
	if err == nil {
		t.Fatal("EnsureDirWithPerm(\"\") 期望错误，但没有返回错误")
	}
	if !errors.Is(err, ErrEmptyPath) {
		t.Errorf("EnsureDirWithPerm(\"\") 错误 = %v, errors.Is(ErrEmptyPath) = false", err)
	}

	err = EnsureDir("")
	if err == nil {
		t.Fatal("EnsureDir(\"\") 期望错误，但没有返回错误")
	}
	if !errors.Is(err, ErrEmptyPath) {
		t.Errorf("EnsureDir(\"\") 错误 = %v, errors.Is(ErrEmptyPath) = false", err)
	}
}

func TestEnsureDirWithPermNullByte(t *testing.T) {
	err := EnsureDirWithPerm("/var/log/\x00app.log", 0755)
	if err == nil {
		t.Fatal("EnsureDirWithPerm(含空字节) 期望错误，但没有返回错误")
	}
	if !errors.Is(err, ErrNullByte) {
		t.Errorf("EnsureDirWithPerm(含空字节) 错误 = %v, errors.Is(ErrNullByte) = false", err)
	}

	// 通过 EnsureDir 间接测试
	err = EnsureDir("/var/log/\x00app.log")
	if err == nil {
		t.Fatal("EnsureDir(含空字节) 期望错误，但没有返回错误")
	}
	if !errors.Is(err, ErrNullByte) {
		t.Errorf("EnsureDir(含空字节) 错误 = %v, errors.Is(ErrNullByte) = false", err)
	}
}

func TestEnsureDirWithPermMkdirFailure(t *testing.T) {
	// 使用普通文件作为父路径，使 os.MkdirAll 在其下创建子目录时失败。
	// 比 chmod 0500 方案更稳定：root/CAP_DAC_OVERRIDE 环境下权限检查可能不生效，
	// 但"父路径是普通文件"在任何权限环境下都会导致 MkdirAll 失败。
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("block"), 0600); err != nil {
		t.Fatalf("创建阻塞文件失败: %v", err)
	}

	err := EnsureDirWithPerm(filepath.Join(blockingFile, "subdir", "app.log"), 0750)
	if err == nil {
		t.Fatal("期望 os.MkdirAll 失败，但没有返回错误")
	}
	// 验证错误包含 xfile 上下文
	if !strings.Contains(err.Error(), "xfile: create directory") {
		t.Errorf("错误消息缺少 xfile 上下文: %v", err)
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
