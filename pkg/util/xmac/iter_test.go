package xmac

import (
	"maps"
	"slices"
	"testing"
)

func TestRange(t *testing.T) {
	tests := []struct {
		name      string
		from      Addr
		to        Addr
		wantCount int
		wantFirst Addr
		wantLast  Addr
	}{
		{
			name:      "single address",
			from:      MustParse("00:00:00:00:00:01"),
			to:        MustParse("00:00:00:00:00:01"),
			wantCount: 1,
			wantFirst: MustParse("00:00:00:00:00:01"),
			wantLast:  MustParse("00:00:00:00:00:01"),
		},
		{
			name:      "small range",
			from:      MustParse("00:00:00:00:00:01"),
			to:        MustParse("00:00:00:00:00:05"),
			wantCount: 5,
			wantFirst: MustParse("00:00:00:00:00:01"),
			wantLast:  MustParse("00:00:00:00:00:05"),
		},
		{
			name:      "cross byte boundary",
			from:      MustParse("00:00:00:00:00:fe"),
			to:        MustParse("00:00:00:00:01:02"),
			wantCount: 5,
			wantFirst: MustParse("00:00:00:00:00:fe"),
			wantLast:  MustParse("00:00:00:00:01:02"),
		},
		{
			name:      "empty range (from > to)",
			from:      MustParse("00:00:00:00:00:05"),
			to:        MustParse("00:00:00:00:00:01"),
			wantCount: 0,
		},
		{
			name:      "zero address",
			from:      Addr{},
			to:        MustParse("00:00:00:00:00:02"),
			wantCount: 3,
			wantFirst: Addr{},
			wantLast:  MustParse("00:00:00:00:00:02"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collected := slices.Collect(Range(tt.from, tt.to))

			if len(collected) != tt.wantCount {
				t.Errorf("Range() count = %d, want %d", len(collected), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if collected[0] != tt.wantFirst {
					t.Errorf("Range() first = %v, want %v", collected[0], tt.wantFirst)
				}
				if collected[len(collected)-1] != tt.wantLast {
					t.Errorf("Range() last = %v, want %v", collected[len(collected)-1], tt.wantLast)
				}
			}
		})
	}
}

func TestRange_EarlyBreak(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:ff")

	count := 0
	for range Range(from, to) {
		count++
		if count >= 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("early break: got %d iterations, want 5", count)
	}
}

func TestRange_Overflow(t *testing.T) {
	// 从广播地址前一个开始
	from := MustParse("ff:ff:ff:ff:ff:fe")
	to := Broadcast()

	collected := slices.Collect(Range(from, to))

	if len(collected) != 2 {
		t.Errorf("overflow range: got %d, want 2", len(collected))
	}
}

func TestRangeN(t *testing.T) {
	tests := []struct {
		name      string
		start     Addr
		n         int
		wantCount int
		wantFirst Addr
		wantLast  Addr
	}{
		{
			name:      "n=5",
			start:     MustParse("00:00:00:00:00:01"),
			n:         5,
			wantCount: 5,
			wantFirst: MustParse("00:00:00:00:00:01"),
			wantLast:  MustParse("00:00:00:00:00:05"),
		},
		{
			name:      "n=1",
			start:     MustParse("aa:bb:cc:dd:ee:ff"),
			n:         1,
			wantCount: 1,
			wantFirst: MustParse("aa:bb:cc:dd:ee:ff"),
			wantLast:  MustParse("aa:bb:cc:dd:ee:ff"),
		},
		{
			name:      "n=0",
			start:     MustParse("00:00:00:00:00:01"),
			n:         0,
			wantCount: 0,
		},
		{
			name:      "n<0",
			start:     MustParse("00:00:00:00:00:01"),
			n:         -1,
			wantCount: 0,
		},
		{
			name:      "overflow before n",
			start:     MustParse("ff:ff:ff:ff:ff:fe"),
			n:         10,
			wantCount: 2, // 只能迭代 fe, ff
			wantFirst: MustParse("ff:ff:ff:ff:ff:fe"),
			wantLast:  Broadcast(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collected := slices.Collect(RangeN(tt.start, tt.n))

			if len(collected) != tt.wantCount {
				t.Errorf("RangeN() count = %d, want %d", len(collected), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if collected[0] != tt.wantFirst {
					t.Errorf("RangeN() first = %v, want %v", collected[0], tt.wantFirst)
				}
				if collected[len(collected)-1] != tt.wantLast {
					t.Errorf("RangeN() last = %v, want %v", collected[len(collected)-1], tt.wantLast)
				}
			}
		})
	}
}

func TestRangeN_EarlyBreak(t *testing.T) {
	start := MustParse("00:00:00:00:00:01")

	count := 0
	for range RangeN(start, 100) {
		count++
		if count >= 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("early break: got %d iterations, want 5", count)
	}
}

func TestRangeWithIndex(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:05")

	// 使用 maps.Collect 收集
	indexToAddr := maps.Collect(RangeWithIndex(from, to))

	if len(indexToAddr) != 5 {
		t.Errorf("RangeWithIndex() count = %d, want 5", len(indexToAddr))
	}

	for i := range 5 {
		if _, ok := indexToAddr[i]; !ok {
			t.Errorf("RangeWithIndex() missing index %d", i)
		}
	}
}

func TestCollectN(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:0a")

	// 收集全部
	all := CollectN(Range(from, to), 0)
	if len(all) != 10 {
		t.Errorf("CollectN(0) = %d, want 10", len(all))
	}

	// 限制数量
	limited := CollectN(Range(from, to), 5)
	if len(limited) != 5 {
		t.Errorf("CollectN(5) = %d, want 5", len(limited))
	}
}

func TestCollectN_LargeMaxCount(t *testing.T) {
	// 验证极大 maxCount 不会导致 OOM（预分配被限制为 1<<20）
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:05")

	// maxCount 远大于实际范围，不应 panic
	result := CollectN(Range(from, to), 1<<30)
	if len(result) != 5 {
		t.Errorf("CollectN(1<<30) = %d, want 5", len(result))
	}
}

func TestCount(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:0a")

	count := Count(Range(from, to))
	if count != 10 {
		t.Errorf("Count() = %d, want 10", count)
	}
}

func TestRangeCount(t *testing.T) {
	tests := []struct {
		name string
		from Addr
		to   Addr
		want uint64
	}{
		{
			name: "single",
			from: MustParse("00:00:00:00:00:01"),
			to:   MustParse("00:00:00:00:00:01"),
			want: 1,
		},
		{
			name: "small range",
			from: MustParse("00:00:00:00:00:01"),
			to:   MustParse("00:00:00:00:00:0a"),
			want: 10,
		},
		{
			name: "cross byte",
			from: MustParse("00:00:00:00:00:00"),
			to:   MustParse("00:00:00:00:01:ff"),
			want: 512,
		},
		{
			name: "empty (from > to)",
			from: MustParse("00:00:00:00:00:0a"),
			to:   MustParse("00:00:00:00:00:01"),
			want: 0,
		},
		{
			name: "full /24 equivalent",
			from: MustParse("aa:bb:cc:00:00:00"),
			to:   MustParse("aa:bb:cc:ff:ff:ff"),
			want: 16777216, // 2^24
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RangeCount(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("RangeCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAddrToUint64(t *testing.T) {
	tests := []struct {
		addr Addr
		want uint64
	}{
		{Addr{}, 0},
		{MustParse("00:00:00:00:00:01"), 1},
		{MustParse("00:00:00:00:01:00"), 256},
		{MustParse("00:00:00:01:00:00"), 65536},
		{MustParse("ff:ff:ff:ff:ff:ff"), 0xFFFFFFFFFFFF}, // 2^48 - 1
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			got := addrToUint64(tt.addr)
			if got != tt.want {
				t.Errorf("addrToUint64(%v) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Benchmark
// =============================================================================

func BenchmarkRange(b *testing.B) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:64") // 100 addresses

	b.ResetTimer()
	for b.Loop() {
		for range Range(from, to) {
		}
	}
}

func BenchmarkRangeN(b *testing.B) {
	start := MustParse("00:00:00:00:00:01")

	b.ResetTimer()
	for b.Loop() {
		for range RangeN(start, 100) {
		}
	}
}

func BenchmarkRangeCount(b *testing.B) {
	from := MustParse("00:00:00:00:00:00")
	to := MustParse("ff:ff:ff:ff:ff:ff")

	b.ResetTimer()
	for b.Loop() {
		_ = RangeCount(from, to)
	}
}

func BenchmarkCollectN(b *testing.B) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:64")

	b.ResetTimer()
	for b.Loop() {
		_ = CollectN(Range(from, to), 0)
	}
}

// =============================================================================
// RangeReverse Tests
// =============================================================================

func TestRangeReverse(t *testing.T) {
	tests := []struct {
		name      string
		from      Addr
		to        Addr
		wantCount int
		wantFirst Addr
		wantLast  Addr
	}{
		{
			name:      "single address",
			from:      MustParse("00:00:00:00:00:01"),
			to:        MustParse("00:00:00:00:00:01"),
			wantCount: 1,
			wantFirst: MustParse("00:00:00:00:00:01"),
			wantLast:  MustParse("00:00:00:00:00:01"),
		},
		{
			name:      "small range reverse",
			from:      MustParse("00:00:00:00:00:01"),
			to:        MustParse("00:00:00:00:00:05"),
			wantCount: 5,
			wantFirst: MustParse("00:00:00:00:00:05"), // 反向：从 to 开始
			wantLast:  MustParse("00:00:00:00:00:01"), // 反向：到 from 结束
		},
		{
			name:      "cross byte boundary reverse",
			from:      MustParse("00:00:00:00:00:fe"),
			to:        MustParse("00:00:00:00:01:02"),
			wantCount: 5,
			wantFirst: MustParse("00:00:00:00:01:02"),
			wantLast:  MustParse("00:00:00:00:00:fe"),
		},
		{
			name:      "empty range (from > to)",
			from:      MustParse("00:00:00:00:00:05"),
			to:        MustParse("00:00:00:00:00:01"),
			wantCount: 0,
		},
		{
			name:      "includes zero address",
			from:      Addr{},
			to:        MustParse("00:00:00:00:00:02"),
			wantCount: 3,
			wantFirst: MustParse("00:00:00:00:00:02"),
			wantLast:  Addr{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collected := slices.Collect(RangeReverse(tt.from, tt.to))

			if len(collected) != tt.wantCount {
				t.Errorf("RangeReverse() count = %d, want %d", len(collected), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if collected[0] != tt.wantFirst {
					t.Errorf("RangeReverse() first = %v, want %v", collected[0], tt.wantFirst)
				}
				if collected[len(collected)-1] != tt.wantLast {
					t.Errorf("RangeReverse() last = %v, want %v", collected[len(collected)-1], tt.wantLast)
				}
			}
		})
	}
}

func TestRangeReverse_EarlyBreak(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:ff")

	count := 0
	for range RangeReverse(from, to) {
		count++
		if count >= 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("early break: got %d iterations, want 5", count)
	}
}

func TestRangeReverse_Underflow(t *testing.T) {
	// 从零地址开始的范围
	from := Addr{}
	to := MustParse("00:00:00:00:00:01")

	collected := slices.Collect(RangeReverse(from, to))

	if len(collected) != 2 {
		t.Errorf("underflow range: got %d, want 2", len(collected))
	}
	// 验证顺序：01, 00
	if collected[0] != MustParse("00:00:00:00:00:01") {
		t.Errorf("first should be 01, got %v", collected[0])
	}
	if collected[1] != (Addr{}) {
		t.Errorf("last should be zero, got %v", collected[1])
	}
}

func TestRangeReverse_ConsistentWithRange(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:0a")

	// 正向收集
	forward := slices.Collect(Range(from, to))
	// 反向收集
	reverse := slices.Collect(RangeReverse(from, to))

	if len(forward) != len(reverse) {
		t.Fatalf("length mismatch: forward=%d, reverse=%d", len(forward), len(reverse))
	}

	// 反向后应该与正向相同
	slices.Reverse(reverse)
	for i := range forward {
		if forward[i] != reverse[i] {
			t.Errorf("mismatch at %d: forward=%v, reverse=%v", i, forward[i], reverse[i])
		}
	}
}

func TestRangeReverseWithIndex(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:05")

	// 使用 maps.Collect 收集
	indexToAddr := maps.Collect(RangeReverseWithIndex(from, to))

	if len(indexToAddr) != 5 {
		t.Errorf("RangeReverseWithIndex() count = %d, want 5", len(indexToAddr))
	}

	// 验证索引 0 对应 to
	if indexToAddr[0] != to {
		t.Errorf("index 0 should be %v, got %v", to, indexToAddr[0])
	}

	// 验证索引 4 对应 from
	if indexToAddr[4] != from {
		t.Errorf("index 4 should be %v, got %v", from, indexToAddr[4])
	}
}

func TestRangeWithIndex_EarlyBreak(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:ff")

	count := 0
	for range RangeWithIndex(from, to) {
		count++
		if count >= 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("early break: got %d iterations, want 5", count)
	}
}

func TestRangeWithIndex_EmptyRange(t *testing.T) {
	from := MustParse("00:00:00:00:00:05")
	to := MustParse("00:00:00:00:00:01")

	count := 0
	for range RangeWithIndex(from, to) {
		count++
	}

	if count != 0 {
		t.Errorf("empty range: got %d iterations, want 0", count)
	}
}

func TestRangeReverseWithIndex_EarlyBreak(t *testing.T) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:ff")

	count := 0
	for range RangeReverseWithIndex(from, to) {
		count++
		if count >= 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("early break: got %d iterations, want 5", count)
	}
}

func TestRangeReverseWithIndex_EmptyRange(t *testing.T) {
	from := MustParse("00:00:00:00:00:05")
	to := MustParse("00:00:00:00:00:01")

	count := 0
	for range RangeReverseWithIndex(from, to) {
		count++
	}

	if count != 0 {
		t.Errorf("empty range: got %d iterations, want 0", count)
	}
}

func BenchmarkRangeReverse(b *testing.B) {
	from := MustParse("00:00:00:00:00:01")
	to := MustParse("00:00:00:00:00:64") // 100 addresses

	b.ResetTimer()
	for b.Loop() {
		for range RangeReverse(from, to) {
		}
	}
}
