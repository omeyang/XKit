package xnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 私有地址
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},

		// IPv4 公网地址
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.15.255.255", false}, // 刚好在 172.16/12 之外
		{"172.32.0.0", false},     // 刚好在 172.16/12 之外

		// IPv6 私有地址 (ULA fc00::/7)
		{"fc00::1", true},
		{"fd00::1", true},
		{"fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", true},

		// IPv6 公网地址
		{"2001:db8::1", false}, // 文档地址，不是私有
		{"::1", false},         // 环回地址

		// IPv4-mapped IPv6 地址（FG-L1: 回归用例）
		{"::ffff:10.0.0.1", true},        // 映射的私网地址
		{"::ffff:192.168.1.1", true},     // 映射的私网地址
		{"::ffff:172.16.0.1", true},      // 映射的私网地址
		{"::ffff:8.8.8.8", false},        // 映射的公网地址
		{"::ffff:172.15.255.255", false}, // 映射的非私网边界

		// 无效地址
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			var addr netip.Addr
			if tt.addr != "" {
				addr = netip.MustParseAddr(tt.addr)
			}
			got := IsPrivate(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 环回
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"127.0.0.0", true},

		// IPv6 环回
		{"::1", true},

		// 非环回
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"::2", false},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsLoopback(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsLinkLocalUnicast(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 链路本地 (169.254.0.0/16)
		{"169.254.0.1", true},
		{"169.254.255.255", true},

		// IPv6 链路本地 (fe80::/10)
		{"fe80::1", true},
		{"fe80::ffff:ffff:ffff:ffff", true},

		// 非链路本地
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsLinkLocalUnicast(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsLinkLocalMulticast(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 链路本地多播 (224.0.0.0/24)
		{"224.0.0.1", true},
		{"224.0.0.251", true}, // mDNS

		// IPv6 链路本地多播 (ff02::/16)
		{"ff02::1", true},

		// 非链路本地多播
		{"224.0.1.1", false},
		{"239.255.255.250", false},
		{"ff01::1", false}, // 接口本地多播，不是链路本地
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsLinkLocalMulticast(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsGlobalUnicast(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// 全局单播（包括私有地址，因为它们也是单播）
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2001:4860:4860::8888", true},
		{"192.168.1.1", true}, // 私有但也是全局单播
		{"10.0.0.1", true},    // 私有但也是全局单播
		{"fc00::1", true},     // ULA 但也是全局单播

		// 非全局单播
		{"224.0.0.1", false},   // 多播
		{"::1", false},         // 环回
		{"0.0.0.0", false},     // 未指定
		{"169.254.0.1", false}, // 链路本地
		{"fe80::1", false},     // 链路本地
		{"127.0.0.1", false},   // 环回
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsGlobalUnicast(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsMulticast(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 多播 (224.0.0.0/4)
		{"224.0.0.1", true},
		{"239.255.255.250", true},

		// IPv6 多播 (ff00::/8)
		{"ff02::1", true},
		{"ff05::2", true},

		// 非多播
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsMulticast(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsUnspecified(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"0.0.0.0", true},
		{"::", true},

		// 非未指定
		{"0.0.0.1", false},
		{"::1", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsUnspecified(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsInterfaceLocalMulticast(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv6 接口本地多播 (ff01::/16)
		{"ff01::1", true},
		{"ff01::2", true},

		// 非接口本地多播
		{"ff02::1", false}, // 链路本地多播
		{"224.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsInterfaceLocalMulticast(addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		addr          string
		wantVersion   Version
		wantString    string
		wantPrivate   bool
		wantLoopback  bool
		wantMulticast bool
		wantGlobal    bool
		wantRoutable  bool
		wantDoc       bool
		wantShared    bool
		wantBenchmark bool
		wantReserved  bool
	}{
		{
			addr:         "192.168.1.1",
			wantVersion:  V4,
			wantString:   "private",
			wantPrivate:  true,
			wantGlobal:   true, // 私有地址也是全局单播
			wantRoutable: true,
		},
		{
			addr:         "127.0.0.1",
			wantVersion:  V4,
			wantString:   "loopback",
			wantLoopback: true,
		},
		{
			addr:          "224.0.0.1",
			wantVersion:   V4,
			wantString:    "link-local-multicast",
			wantMulticast: true,
		},
		{
			addr:         "8.8.8.8",
			wantVersion:  V4,
			wantString:   "global-unicast",
			wantGlobal:   true,
			wantRoutable: true,
		},
		{
			addr:         "2001:4860:4860::8888",
			wantVersion:  V6,
			wantString:   "global-unicast",
			wantGlobal:   true,
			wantRoutable: true,
		},
		{
			addr:         "::1",
			wantVersion:  V6,
			wantString:   "loopback",
			wantLoopback: true,
		},
		// 补充 String() 方法的其他分支覆盖
		{
			addr:        "0.0.0.0",
			wantVersion: V4,
			wantString:  "unspecified",
		},
		{
			addr:        "169.254.1.1",
			wantVersion: V4,
			wantString:  "link-local-unicast",
		},
		{
			addr:          "ff01::1",
			wantVersion:   V6,
			wantString:    "interface-local-multicast",
			wantMulticast: true,
		},
		{
			addr:          "239.255.255.250",
			wantVersion:   V4,
			wantString:    "multicast",
			wantMulticast: true,
		},
		// 新增分类字段覆盖
		{
			addr:         "192.0.2.1",
			wantVersion:  V4,
			wantString:   "documentation",
			wantGlobal:   true,
			wantRoutable: true,
			wantDoc:      true,
		},
		{
			addr:         "100.64.0.1",
			wantVersion:  V4,
			wantString:   "shared-address",
			wantGlobal:   true,
			wantRoutable: true,
			wantShared:   true,
		},
		{
			addr:          "198.18.0.1",
			wantVersion:   V4,
			wantString:    "benchmark",
			wantGlobal:    true,
			wantRoutable:  true,
			wantBenchmark: true,
		},
		{
			addr:          "2001:2::1",
			wantVersion:   V6,
			wantString:    "benchmark",
			wantGlobal:    true,
			wantRoutable:  true,
			wantBenchmark: true,
		},
		{
			addr:         "240.0.0.1",
			wantVersion:  V4,
			wantString:   "reserved",
			wantGlobal:   true,
			wantRoutable: true,
			wantReserved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			c := Classify(addr)

			assert.True(t, c.IsValid)
			assert.Equal(t, tt.wantVersion, c.Version)
			assert.Equal(t, tt.wantString, c.String())
			assert.Equal(t, tt.wantPrivate, c.IsPrivate)
			assert.Equal(t, tt.wantLoopback, c.IsLoopback)
			assert.Equal(t, tt.wantMulticast, c.IsMulticast)
			assert.Equal(t, tt.wantGlobal, c.IsGlobalUnicast)
			assert.Equal(t, tt.wantRoutable, c.IsRoutable)
			assert.Equal(t, tt.wantDoc, c.IsDocumentation)
			assert.Equal(t, tt.wantShared, c.IsSharedAddress)
			assert.Equal(t, tt.wantBenchmark, c.IsBenchmark)
			assert.Equal(t, tt.wantReserved, c.IsReserved)
		})
	}

	// 测试无效地址
	t.Run("invalid", func(t *testing.T) {
		c := Classify(netip.Addr{})
		assert.False(t, c.IsValid)
		assert.Equal(t, "invalid", c.String())
	})

	// 测试 "unknown" 防御性分支：手工构造无标志位的 Classification
	t.Run("unknown branch", func(t *testing.T) {
		c := Classification{IsValid: true}
		assert.Equal(t, "unknown", c.String())
	})
}

func TestIsRoutable(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// 可路由
		{"8.8.8.8", true},
		{"192.168.1.1", true}, // 私有但可路由
		{"10.0.0.1", true},
		{"2001:db8::1", true},

		// 不可路由
		{"127.0.0.1", false},              // 环回
		{"::1", false},                    // 环回
		{"169.254.0.1", false},            // 链路本地
		{"fe80::1", false},                // 链路本地
		{"0.0.0.0", false},                // 未指定
		{"::", false},                     // 未指定
		{"224.0.0.1", false},              // 链路本地多播
		{"ff02::1", false},                // IPv6 链路本地多播
		{"239.255.255.250", false},        // IPv4 多播
		{"255.255.255.255", false},        // IPv4 有限广播
		{"::ffff:255.255.255.255", false}, // IPv4-mapped 广播
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsRoutable(addr)
			assert.Equal(t, tt.want, got)
		})
	}

	// 无效地址
	assert.False(t, IsRoutable(netip.Addr{}))
}

func TestIsDocumentation(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv4 文档地址
		{"192.0.2.1", true}, // TEST-NET-1
		{"192.0.2.255", true},
		{"198.51.100.1", true}, // TEST-NET-2
		{"198.51.100.255", true},
		{"203.0.113.1", true}, // TEST-NET-3
		{"203.0.113.255", true},

		// IPv6 文档地址
		{"2001:db8::1", true},
		{"2001:db8:ffff:ffff:ffff:ffff:ffff:ffff", true},

		// 非文档地址
		{"192.0.3.1", false},
		{"8.8.8.8", false},
		{"192.168.1.1", false},
		{"2001:db7::1", false},
		{"2001:db9::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsDocumentation(addr)
			assert.Equal(t, tt.want, got)
		})
	}

	// 无效地址
	assert.False(t, IsDocumentation(netip.Addr{}))
}

func TestIsSharedAddress(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// 共享地址空间 (100.64.0.0/10)
		{"100.64.0.0", true},
		{"100.64.0.1", true},
		{"100.127.255.255", true},

		// 非共享地址
		{"100.63.255.255", false}, // 刚好在范围外
		{"100.128.0.0", false},    // 刚好在范围外
		{"192.168.1.1", false},
		{"10.0.0.1", false},

		// IPv6 不适用
		{"2001:db8::1", false},

		// IPv4-mapped IPv6 (共享地址空间)
		{"::ffff:100.64.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsSharedAddress(addr)
			assert.Equal(t, tt.want, got)
		})
	}

	// 无效地址
	assert.False(t, IsSharedAddress(netip.Addr{}))
}

func TestIsBenchmark(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// 基准测试地址 (198.18.0.0/15)
		{"198.18.0.0", true},
		{"198.18.0.1", true},
		{"198.19.255.255", true},

		// 非基准测试地址
		{"198.17.255.255", false}, // 刚好在范围外
		{"198.20.0.0", false},     // 刚好在范围外
		{"192.168.1.1", false},

		// IPv6 基准测试地址 (2001:2::/48, RFC 5180)
		{"2001:2::1", true},
		{"2001:2::ffff", true},
		{"2001:2:0:ffff:ffff:ffff:ffff:ffff", true},
		// IPv6 非基准测试地址
		{"2001:db8::1", false},
		{"2001:3::1", false},
		{"2001:1::1", false},

		// IPv4-mapped IPv6 (基准测试地址)
		{"::ffff:198.18.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsBenchmark(addr)
			assert.Equal(t, tt.want, got)
		})
	}

	// 无效地址
	assert.False(t, IsBenchmark(netip.Addr{}))
}

func TestIsReserved(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// 保留地址 (240.0.0.0/4)
		{"240.0.0.0", true},
		{"240.0.0.1", true},
		{"255.255.255.254", true},
		{"255.255.255.255", true}, // 广播地址也在 240.0.0.0/4 范围内

		// 非保留地址
		{"239.255.255.255", false}, // 刚好在范围外（多播最高地址）
		{"192.168.1.1", false},
		{"10.0.0.1", false},

		// IPv6 不适用
		{"2001:db8::1", false},

		// IPv4-mapped IPv6 (保留地址)
		{"::ffff:240.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			got := IsReserved(addr)
			assert.Equal(t, tt.want, got)
		})
	}

	// 无效地址
	assert.False(t, IsReserved(netip.Addr{}))
}

// =============================================================================
// Benchmark
// =============================================================================

func BenchmarkIsPrivate(b *testing.B) {
	addr := netip.MustParseAddr("192.168.1.1")
	for b.Loop() {
		_ = IsPrivate(addr)
	}
}

func BenchmarkClassify(b *testing.B) {
	addr := netip.MustParseAddr("192.168.1.1")
	for b.Loop() {
		_ = Classify(addr)
	}
}

func BenchmarkIsDocumentation(b *testing.B) {
	addr := netip.MustParseAddr("192.0.2.1")
	for b.Loop() {
		_ = IsDocumentation(addr)
	}
}

func BenchmarkIsRoutable(b *testing.B) {
	addr := netip.MustParseAddr("8.8.8.8")
	for b.Loop() {
		_ = IsRoutable(addr)
	}
}

// =============================================================================
// Fuzz
// =============================================================================

func FuzzClassify(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("10.0.0.1")
	f.Add("127.0.0.1")
	f.Add("224.0.0.1")
	f.Add("0.0.0.0")
	f.Add("::1")
	f.Add("2001:db8::1")
	f.Add("fe80::1")
	f.Add("ff02::1")

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}

		c := Classify(addr)

		// 有效地址的分类必须有效
		if addr.IsValid() != c.IsValid {
			t.Errorf("IsValid mismatch: addr=%v, c.IsValid=%v", addr.IsValid(), c.IsValid)
		}

		if c.IsValid {
			assertClassifyFields(t, addr, c)
		}
	})
}

// assertClassifyFields 验证分类结果与直接调用函数的一致性。
func assertClassifyFields(t *testing.T, addr netip.Addr, c Classification) {
	t.Helper()

	if c.IsPrivate != addr.IsPrivate() {
		t.Errorf("IsPrivate mismatch")
	}
	if c.IsLoopback != addr.IsLoopback() {
		t.Errorf("IsLoopback mismatch")
	}
	if c.IsMulticast != addr.IsMulticast() {
		t.Errorf("IsMulticast mismatch")
	}
	if c.IsRoutable != IsRoutable(addr) {
		t.Errorf("IsRoutable mismatch")
	}
	if c.IsDocumentation != IsDocumentation(addr) {
		t.Errorf("IsDocumentation mismatch")
	}
	if c.IsSharedAddress != IsSharedAddress(addr) {
		t.Errorf("IsSharedAddress mismatch")
	}
	if c.IsBenchmark != IsBenchmark(addr) {
		t.Errorf("IsBenchmark mismatch")
	}
	if c.IsReserved != IsReserved(addr) {
		t.Errorf("IsReserved mismatch")
	}
}

// expectedDocumentation 根据 IPv4 uint32 范围判断地址是否为文档地址。
// 仅适用于 IPv4/IPv4-mapped 地址，返回 (expected, applicable)。
func expectedDocumentation(addr netip.Addr) (bool, bool) {
	if !addr.Is4() && !addr.Is4In6() {
		return false, false
	}

	v, ok := AddrToUint32(addr)
	if !ok {
		return false, false
	}

	inTestNet1 := v >= 0xC0000200 && v <= 0xC00002FF
	inTestNet2 := v >= 0xC6336400 && v <= 0xC63364FF
	inTestNet3 := v >= 0xCB007100 && v <= 0xCB0071FF

	return inTestNet1 || inTestNet2 || inTestNet3, true
}

func FuzzIsDocumentation(f *testing.F) {
	// 添加边界值种子
	f.Add("192.0.2.0")    // TEST-NET-1 起始
	f.Add("192.0.2.255")  // TEST-NET-1 结束
	f.Add("192.0.1.255")  // TEST-NET-1 之前
	f.Add("192.0.3.0")    // TEST-NET-1 之后
	f.Add("198.51.100.0") // TEST-NET-2 起始
	f.Add("203.0.113.0")  // TEST-NET-3 起始
	f.Add("2001:db8::1")  // IPv6 文档地址

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}

		got := IsDocumentation(addr)

		// 验证 IPv4 TEST-NET 范围
		if expected, ok := expectedDocumentation(addr); ok {
			if got != expected {
				t.Errorf("IsDocumentation(%s) = %v, expected %v", addr, got, expected)
			}
		}
	})
}
