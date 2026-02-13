package xmac

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// 编译期接口实现检查。
var (
	_ fmt.Stringer             = Addr{}
	_ encoding.TextMarshaler   = Addr{}
	_ encoding.TextUnmarshaler = (*Addr)(nil)
	_ json.Marshaler           = Addr{}
	_ json.Unmarshaler         = (*Addr)(nil)
	_ driver.Valuer            = Addr{}
	_ sql.Scanner              = (*Addr)(nil)
)

func TestAddr_IsValid(t *testing.T) {
	tests := []struct {
		name string
		addr Addr
		want bool
	}{
		{"valid", MustParse("aa:bb:cc:dd:ee:ff"), true},
		{"zero", Addr{}, false},
		{"broadcast", Broadcast(), true},
		{"min_nonzero", Addr{bytes: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_Compare(t *testing.T) {
	tests := []struct {
		name string
		a    Addr
		b    Addr
		want int
	}{
		{"equal", MustParse("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:ff"), 0},
		{"less_first_byte", MustParse("00:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:ff"), -1},
		{"greater_first_byte", MustParse("ff:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:ff"), 1},
		{"less_last_byte", MustParse("aa:bb:cc:dd:ee:00"), MustParse("aa:bb:cc:dd:ee:ff"), -1},
		{"greater_last_byte", MustParse("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:00"), 1},
		{"zero_vs_nonzero", Addr{}, MustParse("00:00:00:00:00:01"), -1},
		{"both_zero", Addr{}, Addr{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Compare(tt.b); got != tt.want {
				t.Errorf("Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_Next(t *testing.T) {
	tests := []struct {
		name    string
		addr    Addr
		want    Addr
		wantErr error
	}{
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ef:00"), nil},
		{"carry", MustParse("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ef:00"), nil},
		{"multi_carry", MustParse("aa:bb:cc:ff:ff:ff"), MustParse("aa:bb:cd:00:00:00"), nil},
		{"from_zero", Addr{}, MustParse("00:00:00:00:00:01"), nil},
		{"overflow", Broadcast(), Addr{}, ErrOverflow},
		{"before_broadcast", MustParse("ff:ff:ff:ff:ff:fe"), Broadcast(), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.addr.Next()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Next() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Next() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Next() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_Prev(t *testing.T) {
	tests := []struct {
		name    string
		addr    Addr
		want    Addr
		wantErr error
	}{
		{"normal", MustParse("aa:bb:cc:dd:ee:ff"), MustParse("aa:bb:cc:dd:ee:fe"), nil},
		{"borrow", MustParse("aa:bb:cc:dd:ef:00"), MustParse("aa:bb:cc:dd:ee:ff"), nil},
		{"multi_borrow", MustParse("aa:bb:cd:00:00:00"), MustParse("aa:bb:cc:ff:ff:ff"), nil},
		{"from_one", MustParse("00:00:00:00:00:01"), Addr{}, nil},
		{"underflow", Addr{}, Addr{}, ErrUnderflow},
		{"from_broadcast", Broadcast(), MustParse("ff:ff:ff:ff:ff:fe"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.addr.Prev()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Prev() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Prev() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Prev() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddr_NextPrev_Inverse(t *testing.T) {
	// Next 和 Prev 互为逆操作
	addrs := []Addr{
		MustParse("00:00:00:00:00:01"),
		MustParse("aa:bb:cc:dd:ee:ff"),
		MustParse("ff:ff:ff:ff:ff:fe"),
	}

	for _, addr := range addrs {
		t.Run(addr.String(), func(t *testing.T) {
			next, err := addr.Next()
			if err != nil {
				t.Fatalf("Next() error = %v", err)
			}
			back, err := next.Prev()
			if err != nil {
				t.Fatalf("Prev() error = %v", err)
			}
			if back != addr {
				t.Errorf("Next().Prev() = %v, want %v", back, addr)
			}
		})
	}
}

func TestAddr_Bytes(t *testing.T) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	bytes := addr.Bytes()
	want := [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

	if bytes != want {
		t.Errorf("Bytes() = %v, want %v", bytes, want)
	}

	// 修改返回值不影响原地址
	bytes[0] = 0x00
	if addr.bytes[0] == 0x00 {
		t.Errorf("Bytes() returned reference instead of copy")
	}
}

func TestAddr_HardwareAddr(t *testing.T) {
	addr := MustParse("aa:bb:cc:dd:ee:ff")
	hw := addr.HardwareAddr()

	if len(hw) != 6 {
		t.Errorf("HardwareAddr() length = %d, want 6", len(hw))
	}

	for i, b := range hw {
		if b != addr.bytes[i] {
			t.Errorf("HardwareAddr()[%d] = %02x, want %02x", i, b, addr.bytes[i])
		}
	}

	// 无效地址返回 nil
	invalid := Addr{}
	if hw := invalid.HardwareAddr(); hw != nil {
		t.Errorf("invalid.HardwareAddr() = %v, want nil", hw)
	}
}

func TestAddrFrom6(t *testing.T) {
	bytes := [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	addr := AddrFrom6(bytes)

	if addr.bytes != bytes {
		t.Errorf("AddrFrom6() = %v, want %v", addr.bytes, bytes)
	}
}
