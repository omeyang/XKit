package xmac

import "testing"

func TestAddr_IsUnicast(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		// 单播地址（第一字节最低位为 0）
		{"unicast_00", MustParse("00:11:22:33:44:55"), true},
		{"unicast_02", MustParse("02:11:22:33:44:55"), true}, // LAA unicast
		{"unicast_fe", MustParse("fe:11:22:33:44:55"), true},

		// 多播地址（第一字节最低位为 1）
		{"multicast_01", MustParse("01:11:22:33:44:55"), false},
		{"multicast_03", MustParse("03:11:22:33:44:55"), false},
		{"multicast_ff", MustParse("ff:11:22:33:44:55"), false},
		{"broadcast", Broadcast, false},

		// 无效地址
		{"invalid", Addr{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsUnicast(); got != tt.want {
				t.Errorf("IsUnicast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsMulticast(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		// 多播地址（第一字节最低位为 1）
		{"multicast_01", MustParse("01:00:5e:00:00:01"), true},
		{"multicast_33", MustParse("33:33:00:00:00:01"), true}, // IPv6 multicast
		{"broadcast", Broadcast, true},

		// 单播地址（第一字节最低位为 0）
		{"unicast_00", MustParse("00:11:22:33:44:55"), false},
		{"unicast_02", MustParse("02:11:22:33:44:55"), false},

		// 无效地址
		{"invalid", Addr{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsMulticast(); got != tt.want {
				t.Errorf("IsMulticast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsBroadcast(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		{"broadcast", Broadcast, true},
		{"not_broadcast", MustParse("ff:ff:ff:ff:ff:fe"), false},
		{"zero", Addr{}, false},
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsBroadcast(); got != tt.want {
				t.Errorf("IsBroadcast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsLocallyAdministered(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		// LAA（第一字节次低位为 1，即 bit 1）
		{"laa_02", MustParse("02:11:22:33:44:55"), true},
		{"laa_03", MustParse("03:11:22:33:44:55"), true},
		{"laa_0a", MustParse("0a:11:22:33:44:55"), true},
		{"laa_fe", MustParse("fe:11:22:33:44:55"), true},

		// UAA（第一字节次低位为 0）
		{"uaa_00", MustParse("00:11:22:33:44:55"), false},
		{"uaa_01", MustParse("01:11:22:33:44:55"), false},
		{"uaa_08", MustParse("08:11:22:33:44:55"), false},
		{"uaa_fc", MustParse("fc:11:22:33:44:55"), false},

		// 无效地址
		{"invalid", Addr{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsLocallyAdministered(); got != tt.want {
				t.Errorf("IsLocallyAdministered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsUniversallyAdministered(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		// UAA（第一字节次低位为 0）
		{"uaa_00", MustParse("00:11:22:33:44:55"), true},
		{"uaa_01", MustParse("01:11:22:33:44:55"), true},
		{"uaa_08", MustParse("08:11:22:33:44:55"), true},

		// LAA（第一字节次低位为 1）
		{"laa_02", MustParse("02:11:22:33:44:55"), false},
		{"laa_03", MustParse("03:11:22:33:44:55"), false},

		// 无效地址
		{"invalid", Addr{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsUniversallyAdministered(); got != tt.want {
				t.Errorf("IsUniversallyAdministered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_IsZero(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		{"zero", Addr{}, true},
		{"zero_explicit", MustParse("00:00:00:00:00:00"), true},
		{"not_zero", MustParse("00:00:00:00:00:01"), false},
		{"broadcast", Broadcast, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_OUI(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want [3]byte
	}{
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), [3]byte{0xaa, 0xbb, 0xcc}},
		{"zero_prefix", MustParse("00:00:00:dd:ee:ff"), [3]byte{0x00, 0x00, 0x00}},
		{"invalid", Addr{}, [3]byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.OUI(); got != tt.want {
				t.Errorf("OUI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_NIC(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want [3]byte
	}{
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), [3]byte{0xdd, 0xee, 0xff}},
		{"zero_suffix", MustParse("aa:bb:cc:00:00:00"), [3]byte{0x00, 0x00, 0x00}},
		{"invalid", Addr{}, [3]byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.NIC(); got != tt.want {
				t.Errorf("NIC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_UnicastMulticast_Mutual_Exclusive(t *testing.T) {
	// 对于有效地址，IsUnicast 和 IsMulticast 互斥
	addrs := []Addr{
		MustParse("00:11:22:33:44:55"),
		MustParse("01:11:22:33:44:55"),
		MustParse("02:11:22:33:44:55"),
		MustParse("03:11:22:33:44:55"),
		Broadcast,
	}

	for _, addr := range addrs {
		unicast := addr.IsUnicast()
		multicast := addr.IsMulticast()

		if unicast && multicast {
			t.Errorf("%v: IsUnicast() and IsMulticast() both true", addr)
		}
		if !unicast && !multicast {
			t.Errorf("%v: IsUnicast() and IsMulticast() both false", addr)
		}
	}
}
