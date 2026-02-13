package xmac

import "testing"

func TestSpecialAddresses(t *testing.T) {
	// 验证预定义的特殊地址
	t.Run("Zero", func(t *testing.T) {
		if Zero() != (Addr{}) {
			t.Errorf("Zero() != Addr{}")
		}
		if Zero().String() != "" {
			t.Errorf("Zero().String() = %q, want empty", Zero().String())
		}
		if Zero().IsValid() {
			t.Errorf("Zero().IsValid() = true, want false")
		}
	})

	t.Run("Broadcast", func(t *testing.T) {
		want := Addr{bytes: [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}}
		if Broadcast() != want {
			t.Errorf("Broadcast() = %v, want %v", Broadcast(), want)
		}
		if Broadcast().String() != "ff:ff:ff:ff:ff:ff" {
			t.Errorf("Broadcast().String() = %q, want ff:ff:ff:ff:ff:ff", Broadcast().String())
		}
		if !Broadcast().IsValid() {
			t.Errorf("Broadcast().IsValid() = false, want true")
		}
	})
}

func TestAddr_IsSpecial(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		{"zero", Zero(), true},
		{"broadcast", Broadcast(), true},
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), false},
		{"almost_zero", MustParse("00:00:00:00:00:01"), false},
		{"almost_broadcast", MustParse("ff:ff:ff:ff:ff:fe"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsSpecial(); got != tt.want {
				t.Errorf("IsSpecial() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsUsable(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		// 可用地址
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), true},
		{"min_usable", MustParse("00:00:00:00:00:01"), true},
		{"max_usable", MustParse("ff:ff:ff:ff:ff:fe"), true},
		{"unicast", MustParse("00:11:22:33:44:55"), true},
		{"multicast", MustParse("01:00:5e:00:00:01"), true},
		{"laa", MustParse("02:11:22:33:44:55"), true},

		// 不可用地址
		{"zero", Zero(), false},
		{"broadcast", Broadcast(), false},
		{"invalid", Addr{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsUsable(); got != tt.want {
				t.Errorf("IsUsable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUsable_BusinessScenario(t *testing.T) {
	// 模拟 go-idt 业务场景中的 MAC 验证
	testCases := []struct {
		name    string
		input   string
		usable  bool
		comment string
	}{
		{"valid_mac", "aa:bb:cc:dd:ee:ff", true, "正常 MAC 地址"},
		{"empty", "", false, "空字符串"},
		{"zero_string", "0", false, "业务约定的无效值（解析失败）"},
		{"zero_mac", "00:00:00:00:00:00", false, "全零 MAC"},
		{"broadcast", "ff:ff:ff:ff:ff:ff", false, "广播地址"},
		{"uppercase", "AA:BB:CC:DD:EE:FF", true, "大写格式"},
		{"dash_format", "aa-bb-cc-dd-ee-ff", true, "短线格式"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := Parse(tc.input)
			usable := err == nil && addr.IsUsable()

			if usable != tc.usable {
				t.Errorf("input=%q: usable=%v, want %v (%s)", tc.input, usable, tc.usable, tc.comment)
			}
		})
	}
}
