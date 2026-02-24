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
// 设计决策: panic 行为说明：
// Go 1.22+ 保证 crypto/rand.Read 永远不返回错误（底层使用 getrandom 等系统调用），
// 如果 OS 熵源不可用，Read 内部会直接崩溃进程。因此此处的 error 检查实际上是
// 不可达的防御性代码，保留是为了与 Read 的 (int, error) 签名保持一致。
func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败表示系统熵源不可用，选择 panic 快速失败
		// 详见上方设计决策说明
		panic("xsampling: crypto/rand.Read failed: " + err.Error())
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>(64-floatBits)) * floatScale
}
