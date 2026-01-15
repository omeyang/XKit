package xsampling

import (
	"math/rand/v2"
)

// randomFloat64 返回 [0.0, 1.0) 范围内的随机浮点数
//
// 使用 math/rand/v2（Go 1.22+），相比 crypto/rand：
//   - 性能提升约 10x（~10ns vs ~100ns）
//   - 线程安全，无需额外锁
//   - 自动使用 ChaCha8 算法初始化
//
// 对于采样场景，统计随机性足够，无需密码学安全随机数。
func randomFloat64() float64 {
	return rand.Float64() //nolint:gosec // G404: 采样场景故意使用 math/rand/v2
}
