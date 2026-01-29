package xmac

import "testing"

func TestAddr_String(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want string
	}{
		{"valid", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, "aa:bb:cc:dd:ee:ff"},
		{"zero", Addr{}, ""},
		{"broadcast", Broadcast, "ff:ff:ff:ff:ff:ff"},
		{"leading_zeros", Addr{bytes: [6]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}}, "01:02:03:04:05:06"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.String(); got != tt.want {
				t.Errorf("Addr.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_FormatString(t *testing.T) {
	addr := Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}

	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{"colon", FormatColon, "aa:bb:cc:dd:ee:ff"},
		{"dash", FormatDash, "aa-bb-cc-dd-ee-ff"},
		{"dot", FormatDot, "aabb.ccdd.eeff"},
		{"bare", FormatBare, "aabbccddeeff"},
		{"colon_upper", FormatColonUpper, "AA:BB:CC:DD:EE:FF"},
		{"dash_upper", FormatDashUpper, "AA-BB-CC-DD-EE-FF"},
		{"unknown_format", Format(255), "aa:bb:cc:dd:ee:ff"}, // 默认为 colon
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addr.FormatString(tt.format); got != tt.want {
				t.Errorf("Addr.FormatString(%v) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}

	// 无效地址测试
	t.Run("invalid_addr", func(t *testing.T) {
		invalid := Addr{}
		for _, f := range []Format{FormatColon, FormatDash, FormatDot, FormatBare} {
			if got := invalid.FormatString(f); got != "" {
				t.Errorf("invalid.FormatString(%v) = %v, want empty", f, got)
			}
		}
	})
}

func TestFormatAllBytes(t *testing.T) {
	// 测试所有可能的字节值都能正确格式化
	// 从 1 开始，因为全零地址是无效的
	for i := 1; i <= 0xff; i++ {
		addr := Addr{bytes: [6]byte{byte(i), byte(i), byte(i), byte(i), byte(i), byte(i)}}
		str := addr.String()
		if len(str) != 17 {
			t.Errorf("String() length = %d, want 17 for byte %02x", len(str), i)
		}

		// 验证往返
		parsed, err := Parse(str)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", str, err)
			continue
		}
		if parsed != addr {
			t.Errorf("round-trip failed for byte %02x: %v != %v", i, parsed, addr)
		}
	}

	// 单独测试全零地址（零值）
	zero := Addr{}
	if zero.String() != "" {
		t.Errorf("zero.String() = %q, want empty", zero.String())
	}
}

func TestFormatLeadingZeros(t *testing.T) {
	// 确保前导零正确保留
	tests := []struct {
		addr Addr
		want string
	}{
		{Addr{bytes: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}, "00:00:00:00:00:01"},
		{Addr{bytes: [6]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00}}, "01:00:00:00:00:00"},
		{Addr{bytes: [6]byte{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}}, "0a:0b:0c:0d:0e:0f"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.addr.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
