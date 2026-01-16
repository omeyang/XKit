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
func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败表示系统随机数源不可用，这是灾难性错误
		panic("xsampling: crypto/rand.Read failed: " + err.Error())
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) * floatScale
}
