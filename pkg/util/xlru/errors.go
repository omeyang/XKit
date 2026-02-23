package xlru

import "errors"

var (
	// ErrInvalidSize 表示缓存大小配置无效。
	ErrInvalidSize = errors.New("xlru: size must be greater than 0")

	// ErrSizeExceedsMax 表示缓存大小超过上限 (16,777,216)。
	ErrSizeExceedsMax = errors.New("xlru: size must not exceed 16777216")

	// ErrInvalidTTL 表示 TTL 配置无效。
	ErrInvalidTTL = errors.New("xlru: TTL must not be negative")
)
