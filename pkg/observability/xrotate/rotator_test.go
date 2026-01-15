package xrotate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 接口兼容性测试
// =============================================================================

// TestRotatorInterface 验证 Rotator 接口定义正确
func TestRotatorInterface(t *testing.T) {
	// 编译时检查：确保接口方法签名正确
	var _ Rotator = (interface {
		Write([]byte) (int, error)
		Close() error
		Rotate() error
	})(nil)
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
		name    string
		setup   func() (string, []LumberjackOption)
		wantErr string
	}{
		{
			name: "空文件名",
			setup: func() (string, []LumberjackOption) {
				return "", nil
			},
			wantErr: "filename is required",
		},
		{
			name: "MaxSizeMB 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxSize(-1)}
			},
			wantErr: "MaxSizeMB must be > 0",
		},
		{
			name: "MaxBackups 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxBackups(-1)}
			},
			wantErr: "MaxBackups must be >= 0",
		},
		{
			name: "MaxAgeDays 为负数",
			setup: func() (string, []LumberjackOption) {
				return "/tmp/test.log", []LumberjackOption{WithMaxAge(-1)}
			},
			wantErr: "MaxAgeDays must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, opts := tt.setup()
			_, err := NewLumberjack(filename, opts...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
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
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

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

	// 等待清理完成
	time.Sleep(100 * time.Millisecond)

	// 检查备份文件数量
	backups, err := findBackups(filename)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(backups), 2, "备份文件数量应该 <= 2")
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

	// 等待压缩完成
	time.Sleep(500 * time.Millisecond)

	// 检查 .gz 文件
	pattern := filename + "-*.gz"
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)

	// 压缩是异步的，可能还没完成
	if len(matches) == 0 {
		backups, _ := findBackups(filename)
		assert.GreaterOrEqual(t, len(backups), 1, "应该有备份文件（压缩中或未压缩）")
	}
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
	r1.Write([]byte("test\n"))
	r1.Rotate()
	r1.Close()

	// 使用 UTC 时间
	filename2 := filepath.Join(tmpDir, "utc.log")
	r2, err := NewLumberjack(filename2,
		WithMaxSize(1),
		WithMaxBackups(5),
		WithCompress(false),
		WithLocalTime(false),
	)
	require.NoError(t, err)
	r2.Write([]byte("test\n"))
	r2.Rotate()
	r2.Close()

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
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Write([]byte("concurrent write\n"))
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证文件存在且有内容
	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
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

// TestWithFileModeExternalChange 测试外部权限变更后的恢复
// 模拟运维人员意外修改了文件权限的场景
func TestWithFileModeExternalChange(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "external_change.log")

	r, err := NewLumberjack(filename,
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

	// 再次写入，应该恢复权限
	_, err = r.Write([]byte("after external change\n"))
	require.NoError(t, err)

	// 验证权限已恢复
	info, err = os.Stat(filename)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "外部权限变更后应恢复为 0600")
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
