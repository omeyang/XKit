package xmac

import (
	"database/sql"
	"encoding/json"
	"net"
	"testing"
)

func BenchmarkParse(b *testing.B) {
	inputs := []struct {
		name  string
		input string
	}{
		{"colon", "aa:bb:cc:dd:ee:ff"},
		{"dash", "aa-bb-cc-dd-ee-ff"},
		{"dot", "aabb.ccdd.eeff"},
		{"bare", "aabbccddeeff"},
	}

	for _, tc := range inputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tc.input)
			}
		})
	}
}

func BenchmarkString(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.String()
	}
}

func BenchmarkFormatString(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")

	formats := []struct {
		name   string
		format Format
	}{
		{"colon", FormatColon},
		{"dash", FormatDash},
		{"dot", FormatDot},
		{"bare", FormatBare},
	}

	for _, tc := range formats {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = addr.FormatString(tc.format)
			}
		})
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(addr)
	}
}

func BenchmarkUnmarshalJSON(b *testing.B) {
	data := []byte(`"aa:bb:cc:dd:ee:ff"`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var addr Addr
		_ = json.Unmarshal(data, &addr)
	}
}

func BenchmarkIsValid(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsValid()
	}
}

func BenchmarkIsUsable(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsUsable()
	}
}

func BenchmarkCompare(b *testing.B) {
	a := MustParse("aa:bb:cc:dd:ee:ff")
	c := MustParse("aa:bb:cc:dd:ee:00")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = a.Compare(c)
	}
}

func BenchmarkNext(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.Next()
	}
}

// =============================================================================
// 新增 Benchmark 测试
// =============================================================================

func BenchmarkPrev(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.Prev()
	}
}

func BenchmarkBytes(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.Bytes()
	}
}

func BenchmarkHardwareAddr(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.HardwareAddr()
	}
}

func BenchmarkOUI(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.OUI()
	}
}

func BenchmarkNIC(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.NIC()
	}
}

// =============================================================================
// 验证方法 Benchmark
// =============================================================================

func BenchmarkIsUnicast(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsUnicast()
	}
}

func BenchmarkIsMulticast(b *testing.B) {
	addr := MustParse("01:00:5e:00:00:01") // multicast address
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsMulticast()
	}
}

func BenchmarkIsBroadcast(b *testing.B) {
	addr := Broadcast
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsBroadcast()
	}
}

func BenchmarkIsLocallyAdministered(b *testing.B) {
	addr := MustParse("02:00:00:00:00:01") // LAA
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsLocallyAdministered()
	}
}

func BenchmarkIsUniversallyAdministered(b *testing.B) {
	addr := MustParse("00:1a:2b:3c:4d:5e") // UAA
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsUniversallyAdministered()
	}
}

func BenchmarkIsZero(b *testing.B) {
	addr := Addr{}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsZero()
	}
}

func BenchmarkIsSpecial(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = addr.IsSpecial()
	}
}

// =============================================================================
// Text 编码 Benchmark
// =============================================================================

func BenchmarkMarshalText(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.MarshalText()
	}
}

func BenchmarkUnmarshalText(b *testing.B) {
	text := []byte("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var addr Addr
		_ = addr.UnmarshalText(text)
	}
}

// =============================================================================
// SQL 接口 Benchmark
// =============================================================================

func BenchmarkValue(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.Value()
	}
}

func BenchmarkScan(b *testing.B) {
	b.Run("string", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var addr Addr
			_ = addr.Scan("aa:bb:cc:dd:ee:ff")
		}
	})

	b.Run("bytes_string", func(b *testing.B) {
		data := []byte("aa:bb:cc:dd:ee:ff")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var addr Addr
			_ = addr.Scan(data)
		}
	})

	b.Run("bytes_binary", func(b *testing.B) {
		data := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var addr Addr
			_ = addr.Scan(data)
		}
	})

	b.Run("nil", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var addr Addr
			_ = addr.Scan(nil)
		}
	})
}

// =============================================================================
// 与 net.HardwareAddr 对比 Benchmark
// =============================================================================

func BenchmarkParseVsNetParseMAC(b *testing.B) {
	input := "aa:bb:cc:dd:ee:ff"

	b.Run("xmac.Parse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = Parse(input)
		}
	})

	b.Run("net.ParseMAC", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = net.ParseMAC(input)
		}
	})
}

func BenchmarkStringVsNetHardwareAddr(b *testing.B) {
	xmacAddr := MustParse("aa:bb:cc:dd:ee:ff")
	netAddr, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

	b.Run("xmac.String", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xmacAddr.String()
		}
	})

	b.Run("net.HardwareAddr.String", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = netAddr.String()
		}
	})
}

func BenchmarkCompareVsBytes(b *testing.B) {
	a := MustParse("aa:bb:cc:dd:ee:ff")
	c := MustParse("aa:bb:cc:dd:ee:00")

	// xmac 使用值类型直接比较
	b.Run("xmac.Compare", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = a.Compare(c)
		}
	})

	// net.HardwareAddr 使用 bytes.Compare
	netA, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	netC, _ := net.ParseMAC("aa:bb:cc:dd:ee:00")
	b.Run("bytes.Equal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = netA.String() == netC.String()
		}
	})
}

// =============================================================================
// 综合场景 Benchmark
// =============================================================================

// BenchmarkTypicalWorkflow 模拟典型业务流程：解析 -> 验证 -> 格式化
func BenchmarkTypicalWorkflow(b *testing.B) {
	input := "AA:BB:CC:DD:EE:FF"
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		addr, err := Parse(input)
		if err != nil {
			b.Fatal(err)
		}
		if !addr.IsUsable() {
			continue
		}
		_ = addr.String()
	}
}

// BenchmarkJSONRoundTrip 测试 JSON 序列化往返
func BenchmarkJSONRoundTrip(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(addr)
		var addr2 Addr
		_ = json.Unmarshal(data, &addr2)
	}
}

// BenchmarkDatabaseRoundTrip 模拟数据库读写往返
func BenchmarkDatabaseRoundTrip(b *testing.B) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// 写入
		val, _ := addr.Value()
		// 读取
		var addr2 Addr
		_ = addr2.Scan(val)
	}
}

// =============================================================================
// 边界情况 Benchmark
// =============================================================================

func BenchmarkParseInvalid(b *testing.B) {
	inputs := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short", "aa:bb:cc"},
		{"invalid_hex", "gg:hh:ii:jj:kk:ll"},
		{"too_long", "aa:bb:cc:dd:ee:ff:00:11"},
	}

	for _, tc := range inputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tc.input)
			}
		})
	}
}

func BenchmarkNextOverflow(b *testing.B) {
	addr := Broadcast // ff:ff:ff:ff:ff:ff
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.Next()
	}
}

func BenchmarkPrevUnderflow(b *testing.B) {
	addr := Addr{} // 00:00:00:00:00:00
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = addr.Prev()
	}
}

// 确保 sql.Scanner 接口实现
var _ sql.Scanner = (*Addr)(nil)
