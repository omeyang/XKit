package xlru

import "errors"

var (
	// ErrInvalidSize 表示缓存大小配置无效。
	ErrInvalidSize = errors.New("xlru: size must be greater than 0")

	// ErrInvalidTTL 表示 TTL 配置无效。
	ErrInvalidTTL = errors.New("xlru: TTL must not be negative")
)
