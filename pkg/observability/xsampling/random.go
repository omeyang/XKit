package xsampling

import "math/rand/v2"

// randomFloat64 返回 [0.0, 1.0) 范围内的伪随机浮点数。
// 采样不属于安全敏感场景，无需使用 crypto/rand。
func randomFloat64() float64 {
	return rand.Float64() // #nosec G404 -- 采样决策不涉及安全用途
}
