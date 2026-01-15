package xlru

import "errors"

var (
	// ErrInvalidSize 表示缓存大小配置无效。
	ErrInvalidSize = errors.New("xlru: size must be greater than 0")
)
