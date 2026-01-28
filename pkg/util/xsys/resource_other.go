//go:build !unix

package xsys

// SetFileLimit 在非 Unix 平台上返回 [ErrUnsupportedPlatform]。
// 参数校验仍然执行，以保持跨平台行为一致。
func SetFileLimit(limit uint64) error {
	if err := validateFileLimit(limit); err != nil {
		return err
	}
	return ErrUnsupportedPlatform
}

// GetFileLimit 在非 Unix 平台上返回 [ErrUnsupportedPlatform]。
func GetFileLimit() (soft, hard uint64, err error) {
	return 0, 0, ErrUnsupportedPlatform
}
