package xsys

// validateFileLimit 校验文件限制值的有效性。
// 跨平台共享校验逻辑，避免 platform-specific 文件中的重复代码。
func validateFileLimit(limit uint64) error {
	if limit == 0 {
		return ErrInvalidFileLimit
	}
	return nil
}
