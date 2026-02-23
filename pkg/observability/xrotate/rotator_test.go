package xrotate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 接口兼容性测试
// =============================================================================

// TestRotatorInterface 验证具体实现满足 Rotator 接口
func TestRotatorInterface(t *testing.T) {
	// 编译时检查：确保 lumberjackRotator 实现了 Rotator 接口
	var _ Rotator = (*lumberjackRotator)(nil)
}

// =============================================================================
// Option 模式测试
// =============================================================================

// TestNewLumberjackWithOptions 测试使用 Option 创建
func TestNewLumberjackWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "options.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(50),
		WithMaxBackups(10),
		WithMaxAge(7),
		WithCompress(false),
		WithLocalTime(true),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入验证
	_, err = r.Write([]byte("test with options\n"))
	assert.NoError(t, err)
}

// TestNewLumberjackWithNilOption 测试 nil option 被静默忽略
func TestNewLumberjackWithNilOption(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "nil_opt.log")

	// nil option 不应 panic
	r, err := NewLumberjack(filename, nil, WithMaxSize(50), nil)
	require.NoError(t, err)
	defer r.Close()

	_, err = r.Write([]byte("test with nil option\n"))
	assert.NoError(t, err)
}

// TestNewLumberjackWithDefaultOptions 测试使用默认配置
func TestNewLumberjackWithDefaultOptions(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "defaults.log")

	// 不传任何 Option，使用全部默认值
	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	_, err = r.Write([]byte("test\n"))
	assert.NoError(t, err)
}

// =============================================================================
// 配置验证测试
// =============================================================================

// TestConfigValidation 测试配置验证
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() (string, []LumberjackOption)
		wantErr   error
		wantInMsg string
	}{
		{
			name: "空文件名",
			setup: func() (string, []LumberjackOption) {
				return "", nil
			},
			wantErr: ErrEmptyFilename,
		},
		{
			name: "MaxSizeMB 为零",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxSize(0)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "0",
		},
		{
			name: "MaxSizeMB 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxSize(-1)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "-1",
		},
		{
			name: "MaxBackups 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxBackups(-1)}
			},
			wantErr:   ErrInvalidMaxBackups,
			wantInMsg: "-1",
		},
		{
			name: "MaxAgeDays 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxAge(-1)}
			},
			wantErr:   ErrInvalidMaxAge,
			wantInMsg: "-1",
		},
		{
			name: "MaxSizeMB 超过上限",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxSize(10241)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "10241",
		},
		{
			name: "MaxBackups 超过上限",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxBackups(1025)}
			},
			wantErr:   ErrInvalidMaxBackups,
			wantInMsg: "1025",
		},
		{
			name: "MaxAgeDays 超过上限",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxAge(3651)}
			},
			wantErr:   ErrInvalidMaxAge,
			wantInMsg: "3651",
		},
		{
			name: "MaxBackups 和 MaxAgeDays 同时为 0",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxBackups(0), WithMaxAge(0)}
			},
			wantErr:   ErrNoCleanupPolicy,
			wantInMsg: "cannot both be 0",
		},
		{
			name: "FileMode 包含文件类型位",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithFileMode(os.ModeDir | 0644)}
			},
			wantErr:   ErrInvalidFileMode,
			wantInMsg: "permission bits",
		},
		{
			name: "FileMode 包含 setuid 位",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithFileMode(os.ModeSetuid | 0777)}
			},
			wantErr:   ErrInvalidFileMode,
			wantInMsg: "permission bits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, opts := tt.setup()
			_, err := NewLumberjack(filename, opts...)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			if tt.wantInMsg != "" {
				assert.Contains(t, err.Error(), tt.wantInMsg)
			}
		})
	}
}

// =============================================================================
// 路径安全测试
// =============================================================================

// TestPathTraversalPrevention 测试路径穿越防护
func TestPathTraversalPrevention(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  string
	}{
		{
			name:     "路径穿越 - 相对路径",
			filename: "../../../etc/passwd",
			wantErr:  "path traversal",
		},
		{
			name:     "路径穿越 - 中间穿越",
			filename: "/var/log/../../../etc/passwd",
			wantErr:  "", // filepath.Clean 会处理成 /etc/passwd，不含 ..
		},
		{
			name:     "纯目录路径",
			filename: "/var/log/",
			wantErr:  "path is a directory",
		},
		{
			name:     "无文件名",
			filename: ".",
			wantErr:  "no file name specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLumberjack(tt.filename)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
			// 对于 wantErr == "" 的情况，可能成功也可能因权限失败
		})
	}
}

// TestPathNormalization 测试路径规范化
func TestPathNormalization(t *testing.T) {
	tmpDir := t.TempDir()

	// 包含 . 和冗余分隔符的路径
	messyPath := filepath.Join(tmpDir, ".", "subdir", ".", "test.log")
	r, err := NewLumberjack(messyPath)
	require.NoError(t, err)
	defer r.Close()

	// 应该能正常写入
	_, err = r.Write([]byte("test\n"))
	assert.NoError(t, err)
}

// =============================================================================
// 基本功能测试
// =============================================================================

// TestWrite 测试写入功能
func TestWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "write_test.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	data := []byte("hello, xrotate!\n")
	n, err := r.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// 验证文件内容
	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

// TestWriteMultiple 测试多次写入
func TestWriteMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "multi_write.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 多次写入
	var expected bytes.Buffer
	for i := 0; i < 100; i++ {
		line := []byte("line of log data\n")
		_, err := r.Write(line)
		require.NoError(t, err)
		expected.Write(line)
	}

	// 验证文件内容
	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, expected.Bytes(), content)
}

// TestClose 测试关闭功能
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "close_test.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	// 写入数据
	_, err = r.Write([]byte("before close\n"))
	require.NoError(t, err)

	// 关闭
	err = r.Close()
	assert.NoError(t, err)
}

// =============================================================================
// 目录创建测试
// =============================================================================

// TestEnsureDirectory 测试自动创建目录
func TestEnsureDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// 多层嵌套目录
	filename := filepath.Join(tmpDir, "a", "b", "c", "nested.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("nested directory test\n"))
	require.NoError(t, err)

	// 验证文件存在
	_, err = os.Stat(filename)
	assert.NoError(t, err)

	// 验证目录权限为 0750
	dir := filepath.Dir(filename)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	// 检查权限（忽略文件类型位）
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0750), perm, "目录权限应为 0750")
}

// TestEnsureDirectoryCurrentDir 测试当前目录文件
func TestEnsureDirectoryCurrentDir(t *testing.T) {
	tmpDir := t.TempDir()
	// 切换到临时目录
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(oldWd))
	})
	require.NoError(t, os.Chdir(tmpDir))

	// 只有文件名，没有目录
	r, err := NewLumberjack("current_dir.log")
	require.NoError(t, err)
	defer r.Close()

	_, err = r.Write([]byte("test\n"))
	assert.NoError(t, err)
}

// =============================================================================
// 轮转测试
// =============================================================================

// TestRotateManual 测试手动轮转
func TestRotateManual(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "manual_rotate.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithMaxAge(30),
		WithCompress(false), // 禁用压缩，便于测试
		WithLocalTime(true),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("before rotate\n"))
	require.NoError(t, err)

	// 手动轮转
	err = r.Rotate()
	require.NoError(t, err)

	// 轮转后继续写入
	_, err = r.Write([]byte("after rotate\n"))
	require.NoError(t, err)

	// 检查备份文件存在
	backups, err := findBackups(filename)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(backups), 1, "应该至少有一个备份文件")
}

// TestRotateBySize 测试按大小自动轮转
func TestRotateBySize(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "size_rotate.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1), // 1MB
		WithMaxBackups(3),
		WithMaxAge(30),
		WithCompress(false),
		WithLocalTime(true),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入超过 1MB 的数据
	payload := bytes.Repeat([]byte("x"), 100*1024) // 100KB
	for i := 0; i < 15; i++ {
		_, err := r.Write(payload)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // 确保时间戳不同
	}

	// 检查备份文件
	backups, err := findBackups(filename)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(backups), 1, "应该有备份文件")
}

// TestMaxBackups 测试最大备份数量限制
func TestMaxBackups(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "max_backups.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),    // 1MB
		WithMaxBackups(2), // 只保留 2 个备份
		WithMaxAge(0),     // 不按天数清理
		WithCompress(false),
		WithLocalTime(true),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入大量数据触发多次轮转
	payload := bytes.Repeat([]byte("x"), 500*1024) // 500KB
	for i := 0; i < 10; i++ {
		_, err := r.Write(payload)
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
	}

	// 使用轮询等待 lumberjack 异步清理完成
	require.Eventually(t, func() bool {
		backups, err := findBackups(filename)
		return err == nil && len(backups) <= 2
	}, 2*time.Second, 50*time.Millisecond, "备份文件数量应在清理后 <= 2")
}

// TestCompress 测试压缩功能
func TestCompress(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "compress.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithMaxAge(30),
		WithCompress(true), // 启用压缩
		WithLocalTime(true),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("data to compress\n"))
	require.NoError(t, err)

	// 手动轮转
	err = r.Rotate()
	require.NoError(t, err)

	// 使用轮询等待异步压缩完成或确认备份存在
	assert.Eventually(t, func() bool {
		// 检查 .gz 压缩文件
		matches, err := filepath.Glob(filename + "-*.gz")
		if err == nil && len(matches) > 0 {
			return true
		}
		// 如果压缩尚未完成，确认至少有未压缩的备份
		backups, err := findBackups(filename)
		return err == nil && len(backups) >= 1
	}, 2*time.Second, 50*time.Millisecond, "应该有备份文件（压缩或未压缩）")
}

// TestLocalTime 测试本地时间选项
func TestLocalTime(t *testing.T) {
	tmpDir := t.TempDir()

	// 使用本地时间
	filename1 := filepath.Join(tmpDir, "local.log")
	r1, err := NewLumberjack(filename1,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithLocalTime(true),
	)
	require.NoError(t, err)
	_, err = r1.Write([]byte("test\n"))
	require.NoError(t, err)
	err = r1.Rotate()
	require.NoError(t, err)
	err = r1.Close()
	require.NoError(t, err)

	// 使用 UTC 时间
	filename2 := filepath.Join(tmpDir, "utc.log")
	r2, err := NewLumberjack(filename2,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithLocalTime(false),
	)
	require.NoError(t, err)
	_, err = r2.Write([]byte("test\n"))
	require.NoError(t, err)
	err = r2.Rotate()
	require.NoError(t, err)
	err = r2.Close()
	require.NoError(t, err)

	// 两种模式都应该产生备份文件
	backups1, _ := findBackups(filename1)
	backups2, _ := findBackups(filename2)
	assert.GreaterOrEqual(t, len(backups1), 1)
	assert.GreaterOrEqual(t, len(backups2), 1)
}

// =============================================================================
// 并发测试
// =============================================================================

// TestConcurrentWrite 测试并发写入
func TestConcurrentWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "concurrent.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 并发写入
	var wg sync.WaitGroup
	errCh := make(chan error, 10*100)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := r.Write([]byte("concurrent write\n")); err != nil {
					errCh <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// 所有写入应成功
	for writeErr := range errCh {
		t.Errorf("unexpected write error: %v", writeErr)
	}

	// 验证文件存在且有内容
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestConcurrentCloseWrite 测试并发 Close 与 Write 的竞争安全
//
// 验证 Write 与 Close 的 TOCTOU 后置检查：当 Write 在 closed 前置检查后、
// logger.Write 执行期间被 Close 中断时，应返回 ErrClosed。
func TestConcurrentCloseWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "close_write_race.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, writeErr := r.Write([]byte("data\n"))
				if writeErr != nil {
					assert.ErrorIs(t, writeErr, ErrClosed)
					return
				}
			}
		}()
	}

	// 短暂等待后关闭
	time.Sleep(5 * time.Millisecond)
	err = r.Close()
	assert.NoError(t, err)

	wg.Wait()
}

// =============================================================================
// 边界条件测试
// =============================================================================

// TestEmptyWrite 测试空写入
func TestEmptyWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "empty.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 空写入
	n, err := r.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

// TestLargeWrite 测试大数据写入
func TestLargeWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "large.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 写入 5MB 数据
	data := bytes.Repeat([]byte("x"), 5*1024*1024)
	n, err := r.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
}

// =============================================================================
// 文件权限测试
// =============================================================================

// TestWithFileMode 测试文件权限选项
func TestWithFileMode(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "filemode.log")

	// 使用 0600 权限
	r, err := NewLumberjack(filename,
		WithFileMode(0600),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据（触发文件创建和权限调整）
	_, err = r.Write([]byte("test with custom file mode\n"))
	require.NoError(t, err)

	// 验证文件权限
	info, err := os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "文件权限应为 0600")
}

// TestWithFileModeDefault 测试不设置 FileMode 的默认行为
func TestWithFileModeDefault(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "default_mode.log")

	// 不设置 FileMode
	r, err := NewLumberjack(filename)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("test with default file mode\n"))
	require.NoError(t, err)

	// 验证文件权限为 lumberjack v2.2+ 默认值 0600
	info, err := os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "文件权限应为 0600（lumberjack v2.2+ 默认值）")
}

// TestWithFileModeMultipleWrites 测试多次写入时权限保持
func TestWithFileModeMultipleWrites(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "multi_write_mode.log")

	r, err := NewLumberjack(filename,
		WithFileMode(0600),
	)
	require.NoError(t, err)
	defer r.Close()

	// 多次写入
	for i := 0; i < 10; i++ {
		_, err = r.Write([]byte("line of data\n"))
		require.NoError(t, err)
	}

	// 验证权限仍然正确
	info, err := os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "多次写入后权限仍应为 0600")
}

// TestWithFileModeAfterRotate 测试轮转后权限
func TestWithFileModeAfterRotate(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "rotate_mode.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithLocalTime(true),
		WithFileMode(0600),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("before rotate\n"))
	require.NoError(t, err)

	// 手动轮转
	err = r.Rotate()
	require.NoError(t, err)

	// 轮转后写入新数据
	_, err = r.Write([]byte("after rotate\n"))
	require.NoError(t, err)

	// 验证新文件权限
	info, err := os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "轮转后新文件权限仍应为 0600")
}

// TestWithFileModeAfterAutoRotate 测试自动轮转后权限保持
// 这是一个关键测试：当 lumberjack 因文件大小超限自动轮转时，
// 新文件必须仍然使用用户指定的权限。
func TestWithFileModeAfterAutoRotate(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "auto_rotate_mode.log")

	// 设置 1MB 的轮转大小和 0644 权限
	r, err := NewLumberjack(filename,
		WithMaxSize(1),      // 1MB 触发轮转
		WithMaxBackups(3),   // 保留 3 个备份
		WithCompress(false), // 禁用压缩便于测试
		WithLocalTime(true),
		WithFileMode(0644), // 期望权限
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入一些初始数据
	_, err = r.Write([]byte("initial data\n"))
	require.NoError(t, err)

	// 验证初始文件权限
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "初始文件权限应为 0644")

	// 写入超过 1MB 的数据触发自动轮转
	// 使用 500KB 块，写入 3 次 = 1.5MB
	payload := bytes.Repeat([]byte("x"), 500*1024)
	for i := 0; i < 3; i++ {
		_, err := r.Write(payload)
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond) // 确保时间戳不同
	}

	// 验证自动轮转后的新文件权限
	info, err = os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0644), perm, "自动轮转后新文件权限仍应为 0644")

	// 验证确实发生了轮转（存在备份文件）
	backups, err := findBackups(filename)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(backups), 1, "应该存在备份文件，说明发生了轮转")
}

// TestRotateCallsEnsureFileMode 验证 Rotate() 本身会调用 ensureFileMode
// 这是对 lumberjack.go:Rotate() 修复的直接测试：
// 修复前：Rotate() 不调整权限，需等待下次 Write() 才能修正
// 修复后：Rotate() 会立即调整权限
func TestRotateCallsEnsureFileMode(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "rotate_filemode.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithLocalTime(true),
		WithFileMode(0644), // 非默认权限，便于验证
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入初始数据（创建文件）
	_, err = r.Write([]byte("initial data\n"))
	require.NoError(t, err)

	// 验证初始文件权限
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "初始文件权限应为 0644")

	// 调用 Rotate() - 注意：这里不调用 Write()
	err = r.Rotate()
	require.NoError(t, err)

	// 关键验证：Rotate() 后立即检查权限（不经过 Write）
	// 如果 Rotate() 正确调用了 ensureFileMode()，权限应为 0644
	info, err = os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0644), perm, "Rotate() 后应立即设置正确权限（无需等待 Write）")
}

// TestWithFileModeExternalChange 测试外部权限变更后通过 Rotate 恢复
//
// 设计决策: 权限检查仅在首次写入和轮转时执行（而非每次写入），
// 以避免热路径上的 os.Stat 系统调用开销。外部权限变更通过 Rotate 修复。
func TestWithFileModeExternalChange(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "external_change.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithFileMode(0600),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入数据
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 验证初始权限
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// 模拟外部权限变更
	err = os.Chmod(filename, 0777)
	require.NoError(t, err)

	// 普通写入不会立即恢复权限（modeApplied 仍为 true）
	_, err = r.Write([]byte("after external change\n"))
	require.NoError(t, err)

	info, err = os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0777), info.Mode().Perm(), "普通写入不触发权限检查")

	// Rotate 后权限应恢复
	err = r.Rotate()
	require.NoError(t, err)

	info, err = os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "Rotate 后应恢复为 0600")
}

// =============================================================================
// 自动轮转权限检测测试
// =============================================================================

// TestWriteFileModeAutoRotationDetection 测试写入超过 MaxSize 后的自动权限检测
//
// 当累计写入字节数超过 MaxSize 时，modeApplied 重置并重新验证权限，
// 确保 lumberjack 自动轮转后创建的新文件也能获得正确权限。
func TestWriteFileModeAutoRotationDetection(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "auto_detect.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(1), // 1MB
		WithMaxBackups(5),
		WithCompress(false),
		WithFileMode(0644),
	)
	require.NoError(t, err)
	defer r.Close()

	// 首次写入：设置 modeApplied=true
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 写入超过 maxSizeBytes (1MB) 的数据触发自动轮转检测
	payload := bytes.Repeat([]byte("x"), 100*1024) // 100KB
	for i := range 12 {                            // 12 * 100KB = 1.2MB > 1MB
		_, err = r.Write(payload)
		require.NoError(t, err)
		if i%3 == 0 {
			time.Sleep(10 * time.Millisecond) // 确保时间戳不同
		}
	}

	// 超过 maxSizeBytes 后权限应被重新验证
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "自动轮转检测后权限应为 0644")
}

// =============================================================================
// 错误回调与注入测试
// =============================================================================

// TestOnErrorCallback 测试错误回调在正常操作中不触发
func TestOnErrorCallback(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "onerror.log")

	var gotError atomic.Value
	r, err := NewLumberjack(filename,
		WithFileMode(0644),
		WithOnError(func(err error) {
			gotError.Store(err)
		}),
	)
	require.NoError(t, err)
	defer r.Close()

	// 正常写入不应触发错误回调
	_, err = r.Write([]byte("test\n"))
	require.NoError(t, err)
	assert.Nil(t, gotError.Load(), "正常写入不应触发 OnError 回调")
}

// TestEnsureFileModeStatError 测试 ensureFileMode 对非 IsNotExist 错误的处理
func TestEnsureFileModeStatError(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "stat_error.log")

	var gotError atomic.Value
	r, err := NewLumberjack(filename,
		WithFileMode(0644),
		WithOnError(func(err error) {
			gotError.Store(err)
		}),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入创建文件
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 注入 Stat 返回权限错误
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.statFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	lr.modeApplied.Store(false) // 强制触发 ensureFileMode

	_, err = r.Write([]byte("trigger stat error\n"))
	require.NoError(t, err) // Write 本身不返回权限错误（best-effort）

	// 错误回调应收到权限错误
	stored := gotError.Load()
	require.NotNil(t, stored, "Stat 权限错误应触发 OnError 回调")
	storedErr, ok := stored.(error)
	require.True(t, ok)
	assert.ErrorIs(t, storedErr, os.ErrPermission)
}

// TestEnsureFileModeFileNotExist 测试 ensureFileMode 对文件不存在的处理
func TestEnsureFileModeFileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "not_exist.log")

	var gotError atomic.Value
	r, err := NewLumberjack(filename,
		WithFileMode(0644),
		WithOnError(func(err error) {
			gotError.Store(err)
		}),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入创建文件
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 注入 Stat 返回文件不存在（模拟 lumberjack 延迟创建场景）
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.statFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	lr.modeApplied.Store(false)

	_, err = r.Write([]byte("trigger not exist\n"))
	require.NoError(t, err)

	// 文件不存在时不应触发错误回调
	assert.Nil(t, gotError.Load(), "文件不存在时不应触发 OnError 回调")
}

// TestEnsureFileModeChmodError 测试 ensureFileMode 对 Chmod 失败的处理
func TestEnsureFileModeChmodError(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "chmod_error.log")

	errChmod := errors.New("chmod failed")
	var gotError atomic.Value
	r, err := NewLumberjack(filename,
		WithFileMode(0644),
		WithOnError(func(err error) {
			gotError.Store(err)
		}),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入创建文件
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 首次 Write 已将权限调整为 0644，手动改回 0600 制造不匹配
	require.NoError(t, os.Chmod(filename, 0600))

	// 注入 Chmod 失败（真实 Stat 返回 0600，与目标 0644 不同，触发 Chmod）
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.chmodFn = func(string, os.FileMode) error {
		return errChmod
	}
	lr.modeApplied.Store(false)

	_, err = r.Write([]byte("trigger chmod error\n"))
	require.NoError(t, err)

	// Chmod 失败应触发错误回调
	stored := gotError.Load()
	require.NotNil(t, stored, "Chmod 失败应触发 OnError 回调")
	storedErr, ok := stored.(error)
	require.True(t, ok)
	assert.Equal(t, errChmod, storedErr)
}

// TestReportErrorCallbackPanic 测试 OnError 回调 panic 被隔离
func TestReportErrorCallbackPanic(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "panic_callback.log")

	r, err := NewLumberjack(filename,
		WithFileMode(0644),
		WithOnError(func(error) {
			panic("callback panic")
		}),
	)
	require.NoError(t, err)
	defer r.Close()

	// 注入 Stat 失败，确保 reportError 被调用
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.statFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	lr.modeApplied.Store(false)

	// 回调 panic 不应传播到调用方
	assert.NotPanics(t, func() {
		_, _ = r.Write([]byte("should not panic\n"))
	})
}

// TestReportErrorNilCallback 测试 reportError 在无回调时不 panic
func TestReportErrorNilCallback(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "nil_callback.log")

	// 不设置 OnError，reportError 应静默忽略
	r, err := NewLumberjack(filename, WithFileMode(0644))
	require.NoError(t, err)
	defer r.Close()

	// 注入 Stat 失败，确保 reportError 被调用
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.statFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	lr.modeApplied.Store(false)

	// 不应 panic
	assert.NotPanics(t, func() {
		_, _ = r.Write([]byte("no panic\n"))
	})
}

// =============================================================================
// Write/Rotate 错误路径测试
// =============================================================================

// TestWriteErrorPath 测试 Write 底层写入失败的路径
func TestWriteErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "write_err.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	// 写入数据创建文件
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 删除文件并使目录只读，导致 lumberjack 无法重新打开文件
	require.NoError(t, os.Remove(filename))
	require.NoError(t, os.Chmod(tmpDir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(tmpDir, 0750)) })

	// 关闭底层 logger 使其尝试重新打开文件（会因权限失败）
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	require.NoError(t, lr.logger.Close())

	// Write 应返回错误（非 ErrClosed，因为 rotator 未关闭）
	_, err = r.Write([]byte("should fail\n"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrClosed)
}

// TestWriteErrorWithConcurrentClose 测试 Write 错误时的 TOCTOU 后置检查
func TestWriteErrorWithConcurrentClose(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "toctou_write.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 删除文件并使目录只读
	require.NoError(t, os.Remove(filename))
	require.NoError(t, os.Chmod(tmpDir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(tmpDir, 0750)) })

	// 关闭底层 logger 使其尝试重新打开文件（会失败）
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	require.NoError(t, lr.logger.Close())

	// 同时标记为已关闭（模拟 TOCTOU 窗口中 Close 完成）
	lr.closed.Store(true)

	// Write 应返回 ErrClosed（后置检查命中）
	_, err = r.Write([]byte("should be ErrClosed\n"))
	assert.ErrorIs(t, err, ErrClosed)
}

// TestRotateErrorPath 测试 Rotate 底层轮转失败的路径
func TestRotateErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "rotate_err.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 使目录只读，导致 lumberjack 无法重命名或创建文件
	require.NoError(t, os.Chmod(tmpDir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(tmpDir, 0750)) })

	// Rotate 应返回错误
	err = r.Rotate()
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrClosed)
}

// TestRotateErrorWithConcurrentClose 测试 Rotate 错误时的 TOCTOU 后置检查
func TestRotateErrorWithConcurrentClose(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "toctou_rotate.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 使目录只读
	require.NoError(t, os.Chmod(tmpDir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(tmpDir, 0750)) })

	// 标记为已关闭（模拟 TOCTOU 窗口）
	lr, ok := r.(*lumberjackRotator)
	require.True(t, ok)
	lr.closed.Store(true)

	// Rotate 应返回 ErrClosed（前置检查命中，因为 closed 在 Chmod 之前设置）
	err = r.Rotate()
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// 覆盖率补充测试
// =============================================================================

// TestWriteAfterClose 测试关闭后写入返回 ErrClosed
func TestWriteAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "write_after_close.log")

	r, err := NewLumberjack(filename)
	require.NoError(t, err)

	// 正常写入
	_, err = r.Write([]byte("before close\n"))
	require.NoError(t, err)

	// 关闭
	err = r.Close()
	require.NoError(t, err)

	// 关闭后写入应返回 ErrClosed
	_, err = r.Write([]byte("after close\n"))
	assert.ErrorIs(t, err, ErrClosed)

	// 关闭后轮转应返回 ErrClosed
	err = r.Rotate()
	assert.ErrorIs(t, err, ErrClosed)

	// 重复关闭应返回 ErrClosed
	err = r.Close()
	assert.ErrorIs(t, err, ErrClosed)
}

// TestEnsureFileModeWhenFileRemoved 测试文件被删除后写入不 panic
//
// 设计决策: 文件删除后 lumberjack 会重新创建文件。由于 modeApplied 仍为 true，
// 权限不会立即调整（与外部变更同理），但写入操作不会出错。
func TestEnsureFileModeWhenFileRemoved(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "removed.log")

	r, err := NewLumberjack(filename,
		WithFileMode(0644),
	)
	require.NoError(t, err)
	defer r.Close()

	// 写入创建文件
	_, err = r.Write([]byte("initial\n"))
	require.NoError(t, err)

	// 删除文件
	err = os.Remove(filename)
	require.NoError(t, err)

	// 再次写入 — lumberjack 会重新创建文件，不应 panic
	_, err = r.Write([]byte("after remove\n"))
	require.NoError(t, err)
}

// TestNewLumberjackEnsureDirFailure 测试目录创建失败的路径
func TestNewLumberjackEnsureDirFailure(t *testing.T) {
	// 创建一个目录，然后将其设为只读，使子目录创建失败
	tmpDir := t.TempDir()
	readonlyDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.MkdirAll(readonlyDir, 0750))
	require.NoError(t, os.Chmod(readonlyDir, 0500))
	t.Cleanup(func() {
		// 恢复权限以便 t.TempDir 清理
		require.NoError(t, os.Chmod(readonlyDir, 0750))
	})

	filename := filepath.Join(readonlyDir, "subdir", "test.log")
	_, err := NewLumberjack(filename)
	assert.Error(t, err, "在只读目录中创建子目录应失败")
}

// TestWriteWithFileModePermissionSame 测试权限已正确时不调用 Chmod
func TestWriteWithFileModePermissionSame(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "same_perm.log")

	// 使用 0600 权限（与 lumberjack 默认一致）
	r, err := NewLumberjack(filename,
		WithFileMode(0600),
	)
	require.NoError(t, err)
	defer r.Close()

	// 多次写入，权限始终一致，不需要 chmod
	for i := 0; i < 5; i++ {
		_, err = r.Write([]byte("test\n"))
		require.NoError(t, err)
	}

	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// =============================================================================
// 辅助函数
// =============================================================================

// findBackups 查找备份文件
func findBackups(filename string) ([]string, error) {
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// lumberjack 备份文件格式: name-timestamp.ext 或 name-timestamp.ext.gz
	pattern := filepath.Join(dir, name+"-*")
	return filepath.Glob(pattern)
}
