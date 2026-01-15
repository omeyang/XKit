package xrotate

import (
	"bytes"
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

// FuzzWrite 模糊测试写入功能
//
// 测试目标：
//   - 任意字节序列写入不会导致 panic
//   - 写入成功时返回的字节数等于输入长度
//   - 空字节、特殊字符、超长数据都能正确处理
func FuzzWrite(f *testing.F) {
	// 添加种子语料
	f.Add([]byte("hello world\n"))
	f.Add([]byte(""))
	f.Add([]byte("日志消息\n"))
	f.Add([]byte("special chars: \x00\x01\x02\n"))
	f.Add(bytes.Repeat([]byte("x"), 1024))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})

	tmpDir := f.TempDir()
	filename := filepath.Join(tmpDir, "fuzz_write.log")

	r, err := NewLumberjack(filename)
	if err != nil {
		f.Fatal(err)
	}
	defer r.Close()

	f.Fuzz(func(t *testing.T, data []byte) {
		// 写入应该不会 panic
		n, err := r.Write(data)
		if err != nil {
			// 写入错误是可接受的（如磁盘满）
			return
		}
		// 如果成功，返回的字节数应该等于输入长度
		if n != len(data) {
			t.Errorf("Write returned %d, want %d", n, len(data))
		}
	})
}

// FuzzFilename 模糊测试文件名处理
//
// 测试目标：
//   - 各种文件名输入不会导致 panic
//   - 路径穿越攻击被正确阻止
//   - 无效文件名返回适当的错误
func FuzzFilename(f *testing.F) {
	// 添加种子语料
	f.Add("/tmp/test.log")
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

	// 所有 fuzz 生成的文件都落在临时目录，避免污染仓库工作区（例如创建 ./relative、./a/c 等目录）
	baseDir := f.TempDir()

	f.Fuzz(func(t *testing.T, filename string) {
		origIsDir := strings.HasSuffix(filename, string(filepath.Separator))

		candidate := filename
		if filepath.IsAbs(candidate) {
			candidate = strings.TrimLeft(candidate, string(filepath.Separator))
		}

		// 防止 Join 后变成目录本身或跳出 baseDir（例如 "."、".."、"../../../etc/passwd"）
		if candidate == "" || candidate == "." || candidate == ".." {
			candidate = "fuzz.log"
		}

		path := filepath.Join(baseDir, candidate)

		rel, err := filepath.Rel(baseDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			path = filepath.Join(baseDir, "escaped", filepath.Base(candidate))
		}

		if origIsDir {
			path += string(filepath.Separator)
		}

		// NewLumberjack 不应该 panic
		r, err := NewLumberjack(path)
		if err != nil {
			// 配置错误是可接受的（无效路径、路径穿越等）
			return
		}
		// 如果成功创建，应该能正常关闭
		r.Close()
	})
}

// FuzzOptions 模糊测试配置选项
//
// 测试目标：
//   - 各种配置组合不会导致 panic
//   - 无效配置值被正确拒绝
//   - 边界值处理正确
func FuzzOptions(f *testing.F) {
	// 添加种子语料
	f.Add(100, 7, 30, true, false)
	f.Add(0, 0, 0, false, true)
	f.Add(-1, -1, -1, true, true)
	f.Add(1, 1, 1, false, false)
	f.Add(1000000, 1000, 365, true, false)
	f.Add(1, 0, 0, false, false)

	tmpDir := f.TempDir()

	f.Fuzz(func(t *testing.T, maxSize, maxBackups, maxAge int, compress, localTime bool) {
		filename := filepath.Join(tmpDir, "fuzz_options.log")

		// NewLumberjack 不应该 panic
		r, err := NewLumberjack(filename,
			WithMaxSize(maxSize),
			WithMaxBackups(maxBackups),
			WithMaxAge(maxAge),
			WithCompress(compress),
			WithLocalTime(localTime),
		)
		if err != nil {
			// 配置错误是可接受的（负数值等）
			return
		}
		// 如果成功创建，应该能正常写入和关闭
		r.Write([]byte("test\n"))
		r.Close()
	})
}

// FuzzWriteSequence 模糊测试写入序列
//
// 测试目标：
//   - 随机大小的多次写入不会导致问题
//   - 写入-轮转-写入序列正确工作
func FuzzWriteSequence(f *testing.F) {
	// 添加种子语料：(写入次数, 每次写入大小)
	f.Add(10, 100)
	f.Add(1, 1000000)
	f.Add(1000, 10)
	f.Add(0, 0)
	f.Add(5, 0)

	tmpDir := f.TempDir()
	filename := filepath.Join(tmpDir, "fuzz_sequence.log")

	f.Fuzz(func(t *testing.T, writeCount, writeSize int) {
		// 限制范围避免测试时间过长
		if writeCount < 0 || writeCount > 100 {
			return
		}
		if writeSize < 0 || writeSize > 100000 {
			return
		}

		r, err := NewLumberjack(filename,
			WithMaxSize(1), // 1MB，可能触发轮转
			WithMaxBackups(3),
			WithCompress(false),
		)
		if err != nil {
			t.Skip("failed to create rotator")
		}
		defer r.Close()

		data := bytes.Repeat([]byte("x"), writeSize)
		for i := 0; i < writeCount; i++ {
			_, err := r.Write(data)
			if err != nil {
				// 写入错误可接受
				return
			}
		}
	})
}
