package xmac

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAddr_MarshalText(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want string
	}{
		{"valid", MustParse("aa:bb:cc:dd:ee:ff"), "aa:bb:cc:dd:ee:ff"},
		{"zero", Addr{}, ""},
		{"broadcast", Broadcast(), "ff:ff:ff:ff:ff:ff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.addr.MarshalText()
			if err != nil {
				t.Errorf("MarshalText() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("MarshalText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddr_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Addr
		wantErr bool
	}{
		{"valid", "aa:bb:cc:dd:ee:ff", MustParse("aa:bb:cc:dd:ee:ff"), false},
		{"uppercase", "AA:BB:CC:DD:EE:FF", MustParse("aa:bb:cc:dd:ee:ff"), false},
		{"empty", "", Addr{}, false},
		{"invalid", "invalid", Addr{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr Addr
			err := addr.UnmarshalText([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && addr != tt.want {
				t.Errorf("UnmarshalText() = %v, want %v", addr, tt.want)
			}
		})
	}
}

func TestAddr_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want string
	}{
		{"valid", MustParse("aa:bb:cc:dd:ee:ff"), `"aa:bb:cc:dd:ee:ff"`},
		{"zero", Addr{}, `""`},
		{"broadcast", Broadcast(), `"ff:ff:ff:ff:ff:ff"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.addr.MarshalJSON()
			if err != nil {
				t.Errorf("MarshalJSON() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAddr_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Addr
		wantErr bool
	}{
		{"valid", `"aa:bb:cc:dd:ee:ff"`, MustParse("aa:bb:cc:dd:ee:ff"), false},
		{"uppercase", `"AA:BB:CC:DD:EE:FF"`, MustParse("aa:bb:cc:dd:ee:ff"), false},
		{"empty", `""`, Addr{}, false},
		{"null", `null`, Addr{}, false},
		{"invalid", `"invalid"`, Addr{}, true},
		{"not_string", `123`, Addr{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr Addr
			err := addr.UnmarshalJSON([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && addr != tt.want {
				t.Errorf("UnmarshalJSON() = %v, want %v", addr, tt.want)
			}
		})
	}
}

func TestAddr_UnmarshalJSON_NullWithWhitespace(t *testing.T) {
	// 验证 JSON null 与各种空白组合的正确处理
	// 注意：Go 的 json.Unmarshal 会自动处理空白，当字符串内容为空时返回零值
	tests := []string{
		`null`,
		` null`,
		"  null  ",
		"\t\nnull",
		"\n  null\n",
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			var addr Addr
			err := json.Unmarshal([]byte(tc), &addr)
			if err != nil {
				t.Errorf("json.Unmarshal(%q) error = %v", tc, err)
				return
			}
			if addr.IsValid() {
				t.Errorf("json.Unmarshal(%q) should return invalid addr, got %v", tc, addr)
			}
		})
	}
}

func TestAddr_JSON_RoundTrip(t *testing.T) {
	type TestStruct struct {
		MAC Addr `json:"mac"`
	}

	tests := []struct {
		name string
		addr Addr
	}{
		{"valid", MustParse("aa:bb:cc:dd:ee:ff")},
		{"zero", Addr{}},
		{"broadcast", Broadcast()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := TestStruct{MAC: tt.addr}

			// Marshal
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			var decoded TestStruct
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if decoded.MAC != original.MAC {
				t.Errorf("round-trip failed: %v != %v", decoded.MAC, original.MAC)
			}
		})
	}
}

func TestAddr_Value(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want any
	}{
		{"valid", MustParse("aa:bb:cc:dd:ee:ff"), "aa:bb:cc:dd:ee:ff"},
		{"zero", Addr{}, nil},
		{"broadcast", Broadcast(), "ff:ff:ff:ff:ff:ff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.addr.Value()
			if err != nil {
				t.Errorf("Value() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    Addr
		wantErr error
	}{
		// string 输入
		{"string_valid", "aa:bb:cc:dd:ee:ff", MustParse("aa:bb:cc:dd:ee:ff"), nil},
		{"string_uppercase", "AA:BB:CC:DD:EE:FF", MustParse("aa:bb:cc:dd:ee:ff"), nil},
		{"string_empty", "", Addr{}, nil},
		{"string_invalid", "invalid", Addr{}, ErrInvalidFormat},

		// []byte 字符串格式
		{"bytes_string", []byte("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:ff"), nil},
		{"bytes_empty", []byte{}, Addr{}, nil},

		// []byte 无效字符串格式
		{"bytes_invalid_string", []byte("not-a-mac"), Addr{}, ErrInvalidFormat},

		// []byte 二进制格式
		{"bytes_binary", []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, MustParse("aa:bb:cc:dd:ee:ff"), nil},
		{"bytes_binary_zero", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, Addr{}, nil},

		// nil 输入
		{"nil", nil, Addr{}, nil},

		// 不支持的类型
		{"int", 123, Addr{}, ErrInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr Addr
			err := addr.Scan(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Scan() unexpected error = %v", err)
				return
			}
			if addr != tt.want {
				t.Errorf("Scan() = %v, want %v", addr, tt.want)
			}
		})
	}
}

func TestAddr_SQL_RoundTrip(t *testing.T) {
	// 模拟 SQL 往返
	addrs := []Addr{
		MustParse("aa:bb:cc:dd:ee:ff"),
		{},
		Broadcast(),
	}

	for _, original := range addrs {
		t.Run(original.String(), func(t *testing.T) {
			// Value (写入)
			val, err := original.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}

			// Scan (读取)
			var scanned Addr
			if err := scanned.Scan(val); err != nil {
				t.Fatalf("Scan() error = %v", err)
			}

			if scanned != original {
				t.Errorf("round-trip failed: %v != %v", scanned, original)
			}
		})
	}
}

func TestAddr_NilReceiver(t *testing.T) {
	t.Run("UnmarshalText", func(t *testing.T) {
		var p *Addr
		err := p.UnmarshalText([]byte("aa:bb:cc:dd:ee:ff"))
		if !errors.Is(err, ErrNilReceiver) {
			t.Errorf("UnmarshalText(nil) error = %v, want ErrNilReceiver", err)
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		var p *Addr
		err := p.UnmarshalJSON([]byte(`"aa:bb:cc:dd:ee:ff"`))
		if !errors.Is(err, ErrNilReceiver) {
			t.Errorf("UnmarshalJSON(nil) error = %v, want ErrNilReceiver", err)
		}
	})

	t.Run("Scan", func(t *testing.T) {
		var p *Addr
		err := p.Scan("aa:bb:cc:dd:ee:ff")
		if !errors.Is(err, ErrNilReceiver) {
			t.Errorf("Scan(nil) error = %v, want ErrNilReceiver", err)
		}
	})
}
