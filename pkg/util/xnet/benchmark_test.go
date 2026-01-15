package xnet

import (
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"testing"

	"go4.org/netipx"
)

// =============================================================================
// IP 地址解析基准测试
// =============================================================================

func BenchmarkParseAddr(b *testing.B) {
	b.Run("netip.ParseAddr", func(b *testing.B) {
		for b.Loop() {
			_, _ = netip.ParseAddr("192.168.1.1")
		}
	})
	b.Run("net.ParseIP", func(b *testing.B) {
		for b.Loop() {
			_ = net.ParseIP("192.168.1.1")
		}
	})
}

// =============================================================================
// IP 地址比较基准测试（对标 gobase MIP）
// =============================================================================

func BenchmarkAddrCompare(b *testing.B) {
	// xnet 直接使用 netip.Addr，无需 MIP 封装
	a := netip.MustParseAddr("192.168.1.1")
	c := netip.MustParseAddr("192.168.1.2")

	b.Run("netip.Addr.Compare", func(b *testing.B) {
		for b.Loop() {
			_ = a.Compare(c)
		}
	})

	// gobase 风格：使用 net.IP
	ip1 := net.ParseIP("192.168.1.1")
	ip2 := net.ParseIP("192.168.1.2")
	b.Run("net.IP.Equal", func(b *testing.B) {
		for b.Loop() {
			_ = ip1.Equal(ip2)
		}
	})
}

func BenchmarkAddrCompareIPv6(b *testing.B) {
	a := netip.MustParseAddr("2001:db8::1")
	c := netip.MustParseAddr("2001:db8::2")

	b.Run("netip.Addr.Compare", func(b *testing.B) {
		for b.Loop() {
			_ = a.Compare(c)
		}
	})

	// gobase 风格：使用 big.Int 比较
	bi1 := AddrToBigInt(a)
	bi2 := AddrToBigInt(c)
	b.Run("big.Int.Cmp", func(b *testing.B) {
		for b.Loop() {
			_ = bi1.Cmp(bi2)
		}
	})
}

// =============================================================================
// BigInt 转换基准测试
// =============================================================================

func BenchmarkAddrToBigInt(b *testing.B) {
	addr := netip.MustParseAddr("192.168.1.1")
	b.Run("IPv4", func(b *testing.B) {
		for b.Loop() {
			_ = AddrToBigInt(addr)
		}
	})

	addr6 := netip.MustParseAddr("2001:db8::1")
	b.Run("IPv6", func(b *testing.B) {
		for b.Loop() {
			_ = AddrToBigInt(addr6)
		}
	})
}

func BenchmarkAddrFromBigInt(b *testing.B) {
	v4 := big.NewInt(0xC0A80101)
	b.Run("V4", func(b *testing.B) {
		for b.Loop() {
			_, _ = AddrFromBigInt(v4, V4)
		}
	})

	v6 := new(big.Int)
	v6.SetString("42540766411282592856903984951653826561", 10) // 2001:db8::1
	b.Run("V6", func(b *testing.B) {
		for b.Loop() {
			_, _ = AddrFromBigInt(v6, V6)
		}
	})
}

// =============================================================================
// IPSet Contains 基准测试（对标 gobase MIPRanges 线性搜索）
// =============================================================================

func BenchmarkIPSetContains(b *testing.B) {
	// 构建 1000 个不重叠范围的 IPSet
	var sb netipx.IPSetBuilder
	for i := range uint32(1000) {
		start := AddrFromUint32(i*256 + 1)
		end := AddrFromUint32(i*256 + 200)
		sb.AddRange(netipx.IPRangeFrom(start, end))
	}
	set, _ := sb.IPSet()

	targets := []netip.Addr{
		AddrFromUint32(500*256 + 100), // middle, found
		AddrFromUint32(999*256 + 100), // near end, found
		AddrFromUint32(0xFFFFFFFF),    // not found
	}

	for _, target := range targets {
		b.Run(fmt.Sprintf("Contains/%s", target), func(b *testing.B) {
			for b.Loop() {
				_ = set.Contains(target)
			}
		})
	}
}

// BenchmarkRangeContains 对比 IPSet.Contains（O(log n)）与线性搜索（O(n)）
// 模拟 gobase MIPRanges.Contains 的线性搜索行为
func BenchmarkRangeContains(b *testing.B) {
	// 构建 100 个范围
	ranges := make([]netipx.IPRange, 100)
	for i := range uint32(100) {
		start := AddrFromUint32(i*256 + 1)
		end := AddrFromUint32(i*256 + 200)
		ranges[i] = netipx.IPRangeFrom(start, end)
	}

	// IPSet O(log n)
	set, _ := IPSetFromRanges(ranges)

	// 目标地址：范围中间
	target := AddrFromUint32(50*256 + 100)

	b.Run("IPSet.Contains/O(log_n)", func(b *testing.B) {
		for b.Loop() {
			_ = set.Contains(target)
		}
	})

	// 模拟 gobase MIPRanges 线性搜索 O(n)
	b.Run("LinearSearch/O(n)", func(b *testing.B) {
		for b.Loop() {
			for _, r := range ranges {
				if r.Contains(target) {
					break
				}
			}
		}
	})
}

// BenchmarkRangeContainsV4 测试 IPv4 优化的范围包含判断
func BenchmarkRangeContainsV4(b *testing.B) {
	from := netip.MustParseAddr("192.168.1.1")
	to := netip.MustParseAddr("192.168.1.100")
	addr := netip.MustParseAddr("192.168.1.50")

	r := netipx.IPRangeFrom(from, to)

	b.Run("RangeContainsV4/uint32", func(b *testing.B) {
		for b.Loop() {
			_ = RangeContainsV4(from, to, addr)
		}
	})

	b.Run("IPRange.Contains", func(b *testing.B) {
		for b.Loop() {
			_ = r.Contains(addr)
		}
	})
}

// =============================================================================
// FullIP 格式化基准测试
// =============================================================================

func BenchmarkFormatFullIPAddr(b *testing.B) {
	b.Run("IPv4", func(b *testing.B) {
		addr := netip.MustParseAddr("192.168.1.1")
		for b.Loop() {
			_ = FormatFullIPAddr(addr)
		}
	})
	b.Run("IPv6", func(b *testing.B) {
		addr := netip.MustParseAddr("2001:db8::1")
		for b.Loop() {
			_ = FormatFullIPAddr(addr)
		}
	})
}

func BenchmarkParseFullIP(b *testing.B) {
	b.Run("IPv4", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseFullIP("192.168.001.001")
		}
	})
	b.Run("IPv6", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseFullIP("00000000000000000000000000000001")
		}
	})
}

func BenchmarkNormalizeIP(b *testing.B) {
	for b.Loop() {
		_, _ = NormalizeIP("192.168.1.1")
	}
}

// =============================================================================
// 范围解析基准测试
// =============================================================================

func BenchmarkParseRange(b *testing.B) {
	b.Run("CIDR", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseRange("192.168.1.0/24")
		}
	})
	b.Run("Range", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseRange("10.0.0.1-10.0.0.100")
		}
	})
	b.Run("Mask", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseRange("192.168.1.0/255.255.255.0")
		}
	})
	b.Run("SingleIP", func(b *testing.B) {
		for b.Loop() {
			_, _ = ParseRange("192.168.1.1")
		}
	})
}

func BenchmarkParseRanges(b *testing.B) {
	strs := make([]string, 1000)
	for i := range uint32(1000) {
		start := AddrFromUint32(i * 200)
		end := AddrFromUint32(i*200 + 250)
		strs[i] = start.String() + "-" + end.String()
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = ParseRanges(strs)
	}
}

// =============================================================================
// WireRange 序列化基准测试
// =============================================================================

func BenchmarkWireRangeFrom(b *testing.B) {
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("192.168.1.100"),
	)
	for b.Loop() {
		_ = WireRangeFrom(r)
	}
}

func BenchmarkWireRangeToIPRange(b *testing.B) {
	w := WireRange{S: "192.168.1.1", E: "192.168.1.100"}
	for b.Loop() {
		_, _ = w.ToIPRange()
	}
}

// =============================================================================
// MergeRanges 基准测试
// =============================================================================

func BenchmarkMergeRanges(b *testing.B) {
	// 构建 100 个有重叠的范围
	ranges := make([]netipx.IPRange, 100)
	for i := range uint32(100) {
		start := AddrFromUint32(i * 200)
		end := AddrFromUint32(i*200 + 250) // 有重叠
		ranges[i] = netipx.IPRangeFrom(start, end)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = MergeRanges(ranges)
	}
}

// =============================================================================
// 综合性能对比：xnet vs gobase 风格
// =============================================================================

// BenchmarkXnetVsGobaseIPCreation 对比 IP 创建性能
// xnet: netip.ParseAddr
// gobase: 创建 MIP 对象（需要额外的 uint32 缓存和 big.Int 惰性初始化）
func BenchmarkXnetVsGobaseIPCreation(b *testing.B) {
	b.Run("xnet/netip.ParseAddr", func(b *testing.B) {
		for b.Loop() {
			_, _ = netip.ParseAddr("192.168.1.1")
		}
	})

	// 模拟 gobase MIP 创建：解析 + uint32 缓存
	b.Run("gobase-style/ParseAddr+uint32cache", func(b *testing.B) {
		for b.Loop() {
			addr, _ := netip.ParseAddr("192.168.1.1")
			_, _ = AddrToUint32(addr) // 模拟 uint32 缓存计算
		}
	})
}

// BenchmarkXnetVsGobaseRangeQuery 对比范围查询性能
// xnet: IPSet.Contains O(log n)
// gobase: MIPRanges.Contains O(n) 线性搜索
func BenchmarkXnetVsGobaseRangeQuery(b *testing.B) {
	// 测试不同规模
	for _, n := range []uint32{10, 100, 1000} {
		ranges := make([]netipx.IPRange, n)
		for i := range n {
			start := AddrFromUint32(i*256 + 1)
			end := AddrFromUint32(i*256 + 200)
			ranges[i] = netipx.IPRangeFrom(start, end)
		}
		set, _ := IPSetFromRanges(ranges)

		// 目标在最后一个范围（最坏情况下线性搜索需要遍历全部）
		target := AddrFromUint32((n-1)*256 + 100)

		b.Run(fmt.Sprintf("xnet/IPSet.Contains/n=%d", n), func(b *testing.B) {
			for b.Loop() {
				_ = set.Contains(target)
			}
		})

		b.Run(fmt.Sprintf("gobase-style/LinearSearch/n=%d", n), func(b *testing.B) {
			for b.Loop() {
				for _, r := range ranges {
					if r.Contains(target) {
						break
					}
				}
			}
		})
	}
}

// =============================================================================
// 新增函数基准测试
// =============================================================================

func BenchmarkMapToIPv6(b *testing.B) {
	addr := netip.MustParseAddr("192.168.1.1")
	for b.Loop() {
		_ = MapToIPv6(addr)
	}
}

func BenchmarkUnmapToIPv4(b *testing.B) {
	addr := netip.MustParseAddr("::ffff:192.168.1.1")
	for b.Loop() {
		_ = UnmapToIPv4(addr)
	}
}

func BenchmarkAddrAdd(b *testing.B) {
	b.Run("IPv4", func(b *testing.B) {
		addr := netip.MustParseAddr("192.168.1.100")
		for b.Loop() {
			_, _ = AddrAdd(addr, 1)
		}
	})
	b.Run("IPv6", func(b *testing.B) {
		addr := netip.MustParseAddr("2001:db8::100")
		for b.Loop() {
			_, _ = AddrAdd(addr, 1)
		}
	})
}

func BenchmarkRangeSize(b *testing.B) {
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.0"),
		netip.MustParseAddr("192.168.1.255"),
	)
	for b.Loop() {
		_ = RangeSize(r)
	}
}

func BenchmarkRangeSizeUint64(b *testing.B) {
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.0"),
		netip.MustParseAddr("192.168.1.255"),
	)
	for b.Loop() {
		_, _ = RangeSizeUint64(r)
	}
}
