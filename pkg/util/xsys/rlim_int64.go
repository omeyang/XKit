//go:build freebsd || dragonfly

package xsys

import "math"

// rlimFromUint64 将用户传入的 uint64 转换为 unix.Rlimit 字段类型。
// 在 FreeBSD/DragonFly 上 Rlimit 字段为 int64，超出 math.MaxInt64 时返回 [ErrFileLimitOverflow]。
func rlimFromUint64(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, ErrFileLimitOverflow
	}
	return int64(v), nil
}

// rlimToUint64 将 unix.Rlimit 字段值转换为 uint64 返回给调用方。
// 负值（理论上不应出现）会被截断为 0，避免回传 uint64 的巨大数字误导调用方。
func rlimToUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}
