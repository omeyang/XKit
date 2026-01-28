package xnet

import (
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"testing"

	"go4.org/netipx"
)

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

func BenchmarkAddrCompare(b *testing.B) {
	a := netip.MustParseAddr("192.168.1.1")
	c := netip.MustParseAddr("192.168.1.2")

	b.Run("netip.Addr.Compare", func(b *testing.B) {
		for b.Loop() {
			_ = a.Compare(c)
		}
	})

	ip1 := net.ParseIP("192.168.1.1")
	ip2 := net.ParseIP("192.168.1.2")
	b.Run("net.IP.Equal", func(b *testing.B) {
		for b.Loop() {
			_ = ip1.Equal(ip2)
		}
	})
}

func BenchmarkAddrToBigInt(b *testing.B) {
	addr := netip.MustParseAddr("192.168.1.1")
	b.Run("AddrToBigInt", func(b *testing.B) {
		for b.Loop() {
			_ = AddrToBigInt(addr)
		}
	})
}

func BenchmarkAddrFromBigInt(b *testing.B) {
	v := big.NewInt(0xC0A80101)
	b.Run("AddrFromBigInt/V4", func(b *testing.B) {
		for b.Loop() {
			_, _ = AddrFromBigInt(v, V4)
		}
	})
}

func BenchmarkIPSetContains(b *testing.B) {
	// Build a set of non-overlapping ranges
	var sb netipx.IPSetBuilder
	for i := uint32(0); i < 1000; i++ {
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
}

func BenchmarkParseRanges(b *testing.B) {
	strs := make([]string, 1000)
	for i := uint32(0); i < 1000; i++ {
		start := AddrFromUint32(i * 200)
		end := AddrFromUint32(i*200 + 250)
		strs[i] = start.String() + "-" + end.String()
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = ParseRanges(strs)
	}
}
