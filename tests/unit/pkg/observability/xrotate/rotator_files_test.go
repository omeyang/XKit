package xrotate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	for range 10 {
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
	for range 3 {
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
	if os.Geteuid() == 0 {
		t.Skip("root 绕过 DAC 权限，无法通过只读目录触发写入失败")
	}
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
	if os.Geteuid() == 0 {
		t.Skip("root 绕过 DAC 权限，无法通过只读目录触发轮转失败")
	}
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
	if os.Geteuid() == 0 {
		t.Skip("root 绕过 DAC 权限，无法通过只读目录触发子目录创建失败")
	}
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
	for range 5 {
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
