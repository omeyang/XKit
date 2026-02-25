package xsampling

import (
	"fmt"
	"hash/fnv"
	"hash/maphash"
	"testing"

	"github.com/cespare/xxhash/v2"
)

// 测试用的典型 key
var testKeys = []string{
	"0af7651916cd43dd8448eb211c80319c", // trace_id (32 hex)
	"tenant-12345",                     // tenant_id
	"user-abc-123-xyz",                 // user_id
	"short",                            // 短字符串
	"a-very-long-key-that-might-be-used-in-some-edge-cases-for-sampling", // 长字符串
}

// =============================================================================
// 对比：maphash.String（Go 1.19+）
// =============================================================================

var maphashSeed = maphash.MakeSeed()

func BenchmarkMaphashString(b *testing.B) {
	for b.Loop() {
		for _, key := range testKeys {
			_ = maphash.String(maphashSeed, key)
		}
	}
}

// =============================================================================
// 对比：hash/fnv（有分配）
// =============================================================================

func BenchmarkFNVStdlib(b *testing.B) {
	for b.Loop() {
		for _, key := range testKeys {
			h := fnv.New64a()
			_, _ = h.Write([]byte(key))
			_ = h.Sum64()
		}
	}
}

// =============================================================================
// 零分配 FNV-1a 手动实现
// =============================================================================

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// fnv64aString 是零分配 FNV-1a 实现，仅用于基准对比和分布测试，非生产代码。
func fnv64aString(s string) uint64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

func BenchmarkFNVManual(b *testing.B) {
	for b.Loop() {
		for _, key := range testKeys {
			_ = fnv64aString(key)
		}
	}
}

// =============================================================================
// xxhash（Prometheus/OpenTelemetry 生态系统使用）
// =============================================================================

func BenchmarkXXHash(b *testing.B) {
	for b.Loop() {
		for _, key := range testKeys {
			_ = xxhash.Sum64String(key)
		}
	}
}

// =============================================================================
// 分布均匀性测试（使用真实场景的 key 模式）
// =============================================================================

func TestHashDistribution(t *testing.T) {
	rates := []struct {
		rate      float64
		numKeys   int
		tolerance float64 // 允许的相对偏差百分比
	}{
		{0.1, 100000, 5},
		{0.01, 1000000, 5},
		{0.001, 1000000, 10}, // 低采样率容许更大偏差
	}

	hashes := []struct {
		name string
		hash func(string) uint64
	}{
		{"maphash", func(s string) uint64 { return maphash.String(maphashSeed, s) }},
		{"fnv64a_manual", fnv64aString},
		{"xxhash", xxhash.Sum64String},
	}

	for _, rc := range rates {
		// 生成真实场景的 key（模拟 trace_id、tenant_id 等）
		keys := make([]string, rc.numKeys)
		for i := range rc.numKeys {
			u := uint64(i) //nolint:gosec // i is always non-negative (loop index)
			h := u * 0x9e3779b97f4a7c15
			keys[i] = fmt.Sprintf("%016x%016x", h, h^0xdeadbeefcafebabe)
		}

		for _, tt := range hashes {
			t.Run(fmt.Sprintf("%s/rate=%.4f", tt.name, rc.rate), func(t *testing.T) {
				sampled := 0
				for _, key := range keys {
					hashValue := tt.hash(key)
					normalized := float64(hashValue) / float64(^uint64(0))
					if normalized < rc.rate {
						sampled++
					}
				}

				actualRate := float64(sampled) / float64(rc.numKeys)
				deviation := (actualRate - rc.rate) / rc.rate * 100

				t.Logf("%s rate=%.4f: sampled %d/%d = %.6f (deviation: %.2f%%)",
					tt.name, rc.rate, sampled, rc.numKeys, actualRate, deviation)

				if deviation < -rc.tolerance || deviation > rc.tolerance {
					t.Errorf("distribution deviation too large: %.2f%% (tolerance: %.0f%%)",
						deviation, rc.tolerance)
				}
			})
		}
	}
}

// =============================================================================
// 跨进程一致性测试（模拟分布式场景）
// =============================================================================

func TestCrossProcessConsistency(t *testing.T) {
	// 测试同一个 key 在不同进程中的采样结果是否一致
	// 这对分布式追踪场景非常重要

	testKey := "0af7651916cd43dd8448eb211c80319c"
	rate := 0.5

	t.Run("确定性哈希（fnv64a）", func(t *testing.T) {
		// FNV 是确定性的，同一 key 总是产生相同的哈希值
		hash1 := fnv64aString(testKey)
		hash2 := fnv64aString(testKey)

		if hash1 != hash2 {
			t.Error("FNV should produce consistent hash for same key")
		}

		// 验证采样决策一致
		normalized1 := float64(hash1) / float64(^uint64(0))
		normalized2 := float64(hash2) / float64(^uint64(0))
		sampled1 := normalized1 < rate
		sampled2 := normalized2 < rate

		if sampled1 != sampled2 {
			t.Error("FNV sampling decision should be consistent")
		}
		t.Logf("FNV: key=%s hash=%x sampled=%v", testKey, hash1, sampled1)
	})

	t.Run("确定性哈希（xxhash）", func(t *testing.T) {
		// xxhash 也是确定性的
		hash1 := xxhash.Sum64String(testKey)
		hash2 := xxhash.Sum64String(testKey)

		if hash1 != hash2 {
			t.Error("xxhash should produce consistent hash for same key")
		}

		normalized1 := float64(hash1) / float64(^uint64(0))
		normalized2 := float64(hash2) / float64(^uint64(0))
		sampled1 := normalized1 < rate
		sampled2 := normalized2 < rate

		if sampled1 != sampled2 {
			t.Error("xxhash sampling decision should be consistent")
		}
		t.Logf("xxhash: key=%s hash=%x sampled=%v", testKey, hash1, sampled1)
	})

	t.Run("随机种子哈希（maphash）", func(t *testing.T) {
		// maphash 使用随机种子，不同进程会产生不同结果
		// 这里模拟两个不同的种子
		seed1 := maphash.MakeSeed()
		seed2 := maphash.MakeSeed()

		hash1 := maphash.String(seed1, testKey)
		hash2 := maphash.String(seed2, testKey)

		// 同一种子应该一致
		if maphash.String(seed1, testKey) != hash1 {
			t.Error("maphash should be consistent with same seed")
		}

		// 不同种子很可能不同（有极小概率相同）
		t.Logf("maphash: seed1_hash=%x, seed2_hash=%x, same=%v",
			hash1, hash2, hash1 == hash2)

		// 注意：这不是一个错误，只是说明 maphash 的特性
		// 在分布式追踪中，这可能导致同一 trace 在不同服务中被不同地采样
	})
}
