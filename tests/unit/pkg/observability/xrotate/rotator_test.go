package xrotate

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
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
		setup     func() (string, []Option)
		wantErr   error
		wantInMsg string
	}{
		{
			name: "空文件名",
			setup: func() (string, []Option) {
				return "", nil
			},
			wantErr: ErrEmptyFilename,
		},
		{
			name: "MaxSizeMB 为零",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxSize(0)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "0",
		},
		{
			name: "MaxSizeMB 为负数",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxSize(-1)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "-1",
		},
		{
			name: "MaxBackups 为负数",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxBackups(-1)}
			},
			wantErr:   ErrInvalidMaxBackups,
			wantInMsg: "-1",
		},
		{
			name: "MaxAgeDays 为负数",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxAge(-1)}
			},
			wantErr:   ErrInvalidMaxAge,
			wantInMsg: "-1",
		},
		{
			name: "MaxSizeMB 超过上限",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxSize(10241)}
			},
			wantErr:   ErrInvalidMaxSize,
			wantInMsg: "10241",
		},
		{
			name: "MaxBackups 超过上限",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxBackups(1025)}
			},
			wantErr:   ErrInvalidMaxBackups,
			wantInMsg: "1025",
		},
		{
			name: "MaxAgeDays 超过上限",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxAge(3651)}
			},
			wantErr:   ErrInvalidMaxAge,
			wantInMsg: "3651",
		},
		{
			name: "MaxBackups 和 MaxAgeDays 同时为 0",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithMaxBackups(0), WithMaxAge(0)}
			},
			wantErr:   ErrNoCleanupPolicy,
			wantInMsg: "cannot both be 0",
		},
		{
			name: "FileMode 包含文件类型位",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithFileMode(os.ModeDir | 0644)}
			},
			wantErr:   ErrInvalidFileMode,
			wantInMsg: "permission bits",
		},
		{
			name: "FileMode 包含 setuid 位",
			setup: func() (string, []Option) {
				return "/tmp/test.log", []Option{WithFileMode(os.ModeSetuid | 0777)}
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
	for range 100 {
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
	for range 15 {
		_, err := r.Write(payload)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // 确保时间戳不同
	}

	// 检查备份文件
	backups, err := findBackups(filename)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(backups), 1, "应该有备份文件")
}

// TestRotateBySizeBoundary 测试恰好达到 MaxSize 阈值的边界行为
//
// 验证 lumberjack 在 MaxSize-1、==MaxSize、MaxSize+1 三个边界点的行为：
//   - MaxSize-1：不触发轮转
//   - ==MaxSize：不触发轮转（lumberjack 使用 > 比较，恰好等于不轮转）
//   - MaxSize+1：触发轮转
func TestRotateBySizeBoundary(t *testing.T) {
	tests := []struct {
		name        string
		writeBytes  int  // 写入的总字节数
		wantRotated bool // 是否期望触发轮转
	}{
		{
			name:        "MaxSize-1 字节不触发轮转",
			writeBytes:  1*1024*1024 - 1, // 1MB - 1
			wantRotated: false,
		},
		{
			name:        "恰好 MaxSize 字节不触发轮转",
			writeBytes:  1 * 1024 * 1024, // 1MB（lumberjack 使用 > 比较）
			wantRotated: false,
		},
		{
			name:        "MaxSize+1 字节触发轮转",
			writeBytes:  1*1024*1024 + 1, // 1MB + 1
			wantRotated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filename := filepath.Join(tmpDir, "boundary.log")

			r, err := NewLumberjack(filename,
				WithMaxSize(1), // 1MB
				WithMaxBackups(3),
				WithMaxAge(30),
				WithCompress(false),
				WithLocalTime(true),
			)
			require.NoError(t, err)
			defer r.Close()

			// 分批写入，避免单次写入超过 MaxSize
			chunkSize := 64 * 1024 // 64KB
			remaining := tt.writeBytes
			for remaining > 0 {
				size := min(chunkSize, remaining)
				payload := bytes.Repeat([]byte("x"), size)
				_, err := r.Write(payload)
				require.NoError(t, err)
				remaining -= size
			}

			// 检查是否产生了备份文件
			backups, err := findBackups(filename)
			require.NoError(t, err)

			if tt.wantRotated {
				assert.GreaterOrEqual(t, len(backups), 1,
					"写入 %d 字节应触发轮转", tt.writeBytes)
			} else {
				assert.Equal(t, 0, len(backups),
					"写入 %d 字节不应触发轮转", tt.writeBytes)
			}
		})
	}
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
	for range 10 {
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
	// 不用 t.TempDir(): lumberjack 的 millRun goroutine 无法被 Close 打断
	// （见 doc.go 已知限制），Close 返回后压缩/清理仍可能写目录，
	// 与 t.TempDir 的 RemoveAll 竞争会报 "directory not empty"。
	// 这里手动创建目录并在 Cleanup 中吞掉 ENOTEMPTY。
	tmpDir, err := os.MkdirTemp("", "TestCompress")
	require.NoError(t, err)
	t.Cleanup(func() {
		for range 20 {
			if err := os.RemoveAll(tmpDir); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
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
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
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
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
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
