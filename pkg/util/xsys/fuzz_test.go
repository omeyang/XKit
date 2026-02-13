package xsys

import "testing"

// FuzzValidateFileLimit 对参数校验逻辑进行模糊测试。
// 使用 validateFileLimit 而非 SetFileLimit，避免修改真实系统 rlimit。
func FuzzValidateFileLimit(f *testing.F) {
	// 添加种子语料
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(1024))
	f.Add(uint64(65536))
	f.Add(uint64(1 << 20))
	f.Add(^uint64(0)) // math.MaxUint64

	f.Fuzz(func(t *testing.T, limit uint64) {
		err := validateFileLimit(limit)

		// 零值必须返回 ErrInvalidFileLimit
		if limit == 0 {
			if err == nil {
				t.Error("validateFileLimit(0) should return error")
			}
			return
		}

		// 非零值必须通过校验
		if err != nil {
			t.Errorf("validateFileLimit(%d) unexpected error: %v", limit, err)
		}
	})
}
