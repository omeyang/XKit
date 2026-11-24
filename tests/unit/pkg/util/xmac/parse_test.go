package xmac

import (
	"errors"
	"net"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Addr
		wantErr error
	}{
		// 冒号格式
		{"colon_lower", "aa:bb:cc:dd:ee:ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"colon_upper", "AA:BB:CC:DD:EE:FF", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"colon_mixed", "Aa:Bb:Cc:Dd:Ee:Ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},

		// 短线格式
		{"dash_lower", "aa-bb-cc-dd-ee-ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"dash_upper", "AA-BB-CC-DD-EE-FF", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},

		// 点格式（Cisco 风格）
		{"dot_lower", "aabb.ccdd.eeff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"dot_upper", "AABB.CCDD.EEFF", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},

		// 无分隔符格式
		{"bare_lower", "aabbccddeeff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"bare_upper", "AABBCCDDEEFF", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"bare_mixed", "AaBbCcDdEeFf", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},

		// 特殊地址
		{"zero", "00:00:00:00:00:00", Addr{}, nil},
		{"broadcast", "ff:ff:ff:ff:ff:ff", Broadcast(), nil},

		// 边界值
		{"min_nonzero", "00:00:00:00:00:01", Addr{bytes: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}, nil},
		{"max_minus_one", "ff:ff:ff:ff:ff:fe", Addr{bytes: [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xfe}}, nil},

		// 带空白
		{"leading_space", "  aa:bb:cc:dd:ee:ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"trailing_space", "aa:bb:cc:dd:ee:ff  ", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"both_space", "  aa:bb:cc:dd:ee:ff  ", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},

		// 错误情况
		{"empty", "", Addr{}, ErrEmpty},
		{"only_space", "   ", Addr{}, ErrEmpty},
		{"too_short", "aa:bb:cc", Addr{}, ErrInvalidFormat},
		{"too_long", "aa:bb:cc:dd:ee:ff:00", Addr{}, ErrInvalidFormat},
		{"eui64", "aa:bb:cc:dd:ee:ff:00:11", Addr{}, ErrInvalidLength}, // EUI-64 不支持
		{"invalid_hex", "gg:hh:ii:jj:kk:ll", Addr{}, ErrInvalidFormat},
		{"invalid_bare_hex", "gghhiijjkkll", Addr{}, ErrInvalidFormat},
		{"partial_invalid", "aa:bb:cc:dd:ee:gg", Addr{}, ErrInvalidFormat},
		{"dot_invalid_hex", "ggbb.ccdd.eeff", Addr{}, ErrInvalidFormat},
		{"wrong_separator", "aa;bb;cc;dd;ee;ff", Addr{}, ErrInvalidFormat},
		{"mixed_separator", "aa:bb-cc:dd-ee:ff", Addr{}, ErrInvalidFormat},
		{"single_digit", "a:b:c:d:e:f", Addr{}, ErrInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Parse(%q) error = nil, wantErr %v", tt.input, tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Parse(%q) unexpected error = %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	// 正常情况
	t.Run("valid", func(t *testing.T) {
		addr := MustParse("aa:bb:cc:dd:ee:ff")
		want := Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
		if addr != want {
			t.Errorf("MustParse() = %v, want %v", addr, want)
		}
	})

	// panic 情况
	t.Run("invalid_panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MustParse(invalid) did not panic")
			}
		}()
		MustParse("invalid")
	})
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    Addr
		wantErr error
	}{
		{"valid", []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"zero", []byte{0, 0, 0, 0, 0, 0}, Addr{}, nil},
		{"broadcast", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, Broadcast(), nil},
		{"too_short", []byte{0xaa, 0xbb, 0xcc}, Addr{}, ErrInvalidLength},
		{"too_long", []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}, Addr{}, ErrInvalidLength},
		{"empty", []byte{}, Addr{}, ErrInvalidLength},
		{"nil", nil, Addr{}, ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBytes(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ParseBytes() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseBytes() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromHardwareAddr(t *testing.T) {
	tests := []struct {
		name    string
		input   net.HardwareAddr
		want    Addr
		wantErr error
	}{
		{"valid", net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}, nil},
		{"too_short", net.HardwareAddr{0xaa, 0xbb, 0xcc}, Addr{}, ErrInvalidLength},
		{"eui64", net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11}, Addr{}, ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromHardwareAddr(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("FromHardwareAddr() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("FromHardwareAddr() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("FromHardwareAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRoundTrip(t *testing.T) {
	// 测试解析-格式化往返（仅测试有效地址）
	inputs := []string{
		"aa:bb:cc:dd:ee:ff",
		"AA:BB:CC:DD:EE:FF",
		"aa-bb-cc-dd-ee-ff",
		"aabb.ccdd.eeff",
		"aabbccddeeff",
		"ff:ff:ff:ff:ff:ff",
		"00:00:00:00:00:01", // 最小有效地址
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			addr, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", input, err)
			}
			// 往返测试
			str := addr.String()
			addr2, err := Parse(str)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v (round-trip)", str, err)
			}
			if addr != addr2 {
				t.Errorf("round-trip failed: %v != %v", addr, addr2)
			}
		})
	}

	// 单独测试全零地址（特殊情况）
	t.Run("zero_address", func(t *testing.T) {
		addr, err := Parse("00:00:00:00:00:00")
		if err != nil {
			t.Fatalf("Parse(zero) error = %v", err)
		}
		if addr.IsValid() {
			t.Errorf("zero address should be invalid")
		}
		if addr.String() != "" {
			t.Errorf("zero.String() = %q, want empty", addr.String())
		}
	})
}

// TestParseStdlib 直接测试 parseStdlib 内部函数的成功路径。
// 通过 Parse() 调用时，所有标准 6 字节 MAC 格式都被自定义解析器拦截，
// parseStdlib 的成功路径（net.ParseMAC 返回 6 字节）不会被覆盖。
func TestParseStdlib(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Addr
	}{
		{"colon", "aa:bb:cc:dd:ee:ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}},
		{"dash", "aa-bb-cc-dd-ee-ff", Addr{bytes: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStdlib(tt.input)
			if err != nil {
				t.Errorf("parseStdlib(%q) error = %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseStdlib(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParse_ZeroAddressConsistency 验证全零 MAC 地址的一致性行为。
// 这是有意的设计决策：Parse("00:00:00:00:00:00") 返回零值 Addr{}，
// 其 IsValid() 返回 false，与 net/netip.Addr 的设计一致。
func TestParse_ZeroAddressConsistency(t *testing.T) {
	// 全零地址字符串解析
	addr, err := Parse("00:00:00:00:00:00")
	if err != nil {
		t.Fatalf("Parse(00:00:00:00:00:00) error = %v", err)
	}

	// 验证返回的是零值
	var zeroAddr Addr
	if addr != zeroAddr {
		t.Errorf("Parse(00:00:00:00:00:00) = %v, want zero value", addr)
	}

	// 验证 IsValid() 返回 false（设计决策）
	if addr.IsValid() {
		t.Errorf("zero address IsValid() = true, want false")
	}

	// 验证 IsUsable() 也返回 false
	if addr.IsUsable() {
		t.Errorf("zero address IsUsable() = true, want false")
	}

	// 验证 String() 返回空字符串
	if addr.String() != "" {
		t.Errorf("zero address String() = %q, want empty", addr.String())
	}

	// 验证与字节数组解析的一致性
	addrFromBytes, err := ParseBytes([]byte{0, 0, 0, 0, 0, 0})
	if err != nil {
		t.Fatalf("ParseBytes(zeros) error = %v", err)
	}
	if addrFromBytes != addr {
		t.Errorf("ParseBytes(zeros) != Parse(00:00:00:00:00:00)")
	}

	// 验证与预定义 Zero 常量的一致性
	if addr != Zero() {
		t.Errorf("Parse(00:00:00:00:00:00) != Zero()")
	}
}
