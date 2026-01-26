package xsampling

import (
	"crypto/rand"
	"encoding/binary"
)

// 浮点数转换常量
const (
	floatBits  = 53
	floatScale = 1.0 / (1 << floatBits)
)

// randomFloat64 返回 [0.0, 1.0) 范围内的随机浮点数
//
// 使用 crypto/rand 确保高质量随机数：
//   - 密码学安全随机数生成器
//   - 适用于所有采样场景
//
// 性能说明：crypto/rand 在现代 CPU 上约 ~50-100ns/op，
// 对于采样场景（通常每请求调用一次）完全可接受。
//
// 设计决策 - panic 行为说明：
// crypto/rand.Read 失败表示操作系统熵源不可用（如 /dev/urandom 无法访问），
// 这是极其罕见的系统级故障。此时继续运行会产生不安全/不可预测的采样行为，
// 可能导致更难排查的问题。因此选择 panic 作为快速失败策略，便于问题定位。
// 在实际生产环境中，此错误几乎不会发生。
func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败表示系统熵源不可用，选择 panic 快速失败
		// 详见上方设计决策说明
		panic("xsampling: crypto/rand.Read failed: " + err.Error())
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) * floatScale
}
