//go:build unix && !freebsd && !dragonfly

package xsys

// rlimFromUint64 将用户传入的 uint64 转换为 unix.Rlimit 字段类型。
// 在大多数 Unix 平台上 Rlimit 字段为 uint64，直接返回即可。
func rlimFromUint64(v uint64) (uint64, error) {
	return v, nil
}

// rlimToUint64 将 unix.Rlimit 字段值转换为 uint64 返回给调用方。
func rlimToUint64(v uint64) uint64 {
	return v
}
