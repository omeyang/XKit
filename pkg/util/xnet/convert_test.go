package xnet

import (
	"math/big"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddrFromUint32(t *testing.T) {
	// 192.168.1.1 = 0xC0A80101
	addr := AddrFromUint32(0xC0A80101)
	assert.Equal(t, "192.168.1.1", addr.String())
	assert.True(t, addr.Is4())

	v, ok := AddrToUint32(addr)
	require.True(t, ok)
	assert.Equal(t, uint32(0xC0A80101), v)

	// 0.0.0.0
	addr = AddrFromUint32(0)
	assert.Equal(t, "0.0.0.0", addr.String())
}

func TestAddrToUint32(t *testing.T) {
	addr := netip.MustParseAddr("192.168.1.1")
	v, ok := AddrToUint32(addr)
	require.True(t, ok)
	assert.Equal(t, uint32(0xC0A80101), v)

	v6 := netip.MustParseAddr("::1")
	_, ok = AddrToUint32(v6)
	assert.False(t, ok)

	// IPv4-mapped IPv6
	mapped := netip.MustParseAddr("::ffff:192.168.1.1")
	v, ok = AddrToUint32(mapped)
	require.True(t, ok)
	assert.Equal(t, uint32(0xC0A80101), v)
}

func TestAddrFromBigInt(t *testing.T) {
	// IPv4
	v4 := big.NewInt(0xC0A80101)
	addr, err := AddrFromBigInt(v4, V4)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", addr.String())

	// IPv6
	v6 := new(big.Int)
	v6.SetString("1", 10) // ::1
	addr, err = AddrFromBigInt(v6, V6)
	require.NoError(t, err)
	assert.Equal(t, "::1", addr.String())

	// Errors
	_, err = AddrFromBigInt(nil, V4)
	assert.ErrorIs(t, err, ErrInvalidBigInt)

	_, err = AddrFromBigInt(big.NewInt(-1), V4)
	assert.ErrorIs(t, err, ErrInvalidBigInt)

	_, err = AddrFromBigInt(new(big.Int).SetUint64(1<<33), V4)
	assert.ErrorIs(t, err, ErrInvalidBigInt)

	_, err = AddrFromBigInt(v4, Version(0))
	assert.ErrorIs(t, err, ErrInvalidVersion)

	// IPv6: 超出 128 位
	tooBig := new(big.Int).Lsh(big.NewInt(1), 129)
	_, err = AddrFromBigInt(tooBig, V6)
	assert.ErrorIs(t, err, ErrInvalidBigInt)

	// IPv6: 128 位最大值（全 1）应成功
	maxV6 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	addr, err = AddrFromBigInt(maxV6, V6)
	require.NoError(t, err)
	assert.Equal(t, "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", addr.String())
}

func TestAddrToBigInt(t *testing.T) {
	// IPv4 round-trip
	addr := netip.MustParseAddr("192.168.1.1")
	bi := AddrToBigInt(addr)
	assert.Equal(t, int64(0xC0A80101), bi.Int64())

	// IPv6
	addr = netip.MustParseAddr("::1")
	bi = AddrToBigInt(addr)
	assert.Equal(t, int64(1), bi.Int64())

	// Zero/invalid value
	bi = AddrToBigInt(netip.Addr{})
	assert.Equal(t, int64(0), bi.Int64())
}

func TestAddrBigIntRoundTrip(t *testing.T) {
	tests := []struct {
		input string
		ver   Version
	}{
		{"0.0.0.0", V4},
		{"192.168.1.1", V4},
		{"255.255.255.255", V4},
		{"::1", V6},
		{"2001:db8::1", V6},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			orig := netip.MustParseAddr(tt.input)
			bi := AddrToBigInt(orig)
			restored, err := AddrFromBigInt(bi, tt.ver)
			require.NoError(t, err)
			assert.Equal(t, 0, orig.Compare(restored), "round-trip failed for %s", tt.input)
		})
	}
}

func TestMapToIPv6(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"IPv4 to mapped", "192.168.1.1", "::ffff:192.168.1.1"},
		{"IPv4 zero", "0.0.0.0", "::ffff:0.0.0.0"},
		{"IPv4 max", "255.255.255.255", "::ffff:255.255.255.255"},
		{"IPv6 unchanged", "2001:db8::1", "2001:db8::1"},
		{"IPv6 loopback", "::1", "::1"},
		{"already mapped", "::ffff:192.168.1.1", "::ffff:192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.input)
			mapped := MapToIPv6(addr)
			assert.Equal(t, tt.want, mapped.String())
		})
	}

	// 无效地址
	invalid := MapToIPv6(netip.Addr{})
	assert.False(t, invalid.IsValid())
}

func TestUnmapToIPv4(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"mapped to IPv4", "::ffff:192.168.1.1", "192.168.1.1"},
		{"pure IPv4 unchanged", "192.168.1.1", "192.168.1.1"},
		{"pure IPv6 unchanged", "2001:db8::1", "2001:db8::1"},
		{"IPv6 loopback unchanged", "::1", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.input)
			unmapped := UnmapToIPv4(addr)
			assert.Equal(t, tt.want, unmapped.String())
		})
	}

	// 无效地址
	invalid := UnmapToIPv4(netip.Addr{})
	assert.False(t, invalid.IsValid())
}

func TestMapUnmapRoundTrip(t *testing.T) {
	// IPv4 → mapped → unmapped 应该回到原始值
	orig := netip.MustParseAddr("192.168.1.1")
	mapped := MapToIPv6(orig)
	assert.True(t, mapped.Is4In6())
	unmapped := UnmapToIPv4(mapped)
	assert.Equal(t, orig, unmapped)
}

func TestAddrAdd(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		delta   int64
		want    string
		wantErr bool
	}{
		{"add 1", "192.168.1.100", 1, "192.168.1.101", false},
		{"add 10", "192.168.1.100", 10, "192.168.1.110", false},
		{"subtract 1", "192.168.1.100", -1, "192.168.1.99", false},
		{"subtract 100", "192.168.1.100", -100, "192.168.1.0", false},
		{"IPv4 boundary", "255.255.255.254", 1, "255.255.255.255", false},
		{"IPv6 add", "::1", 1, "::2", false},
		{"IPv6 subtract", "::ff", -1, "::fe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.input)
			result, err := AddrAdd(addr, tt.delta)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.String())
		})
	}
}

func TestAddrAdd_Overflow(t *testing.T) {
	// IPv4 最大值 + 1 应该溢出
	maxV4 := netip.MustParseAddr("255.255.255.255")
	_, err := AddrAdd(maxV4, 1)
	assert.ErrorIs(t, err, ErrOverflow)

	// IPv4 最小值 - 1 应该溢出
	minV4 := netip.MustParseAddr("0.0.0.0")
	_, err = AddrAdd(minV4, -1)
	assert.ErrorIs(t, err, ErrOverflow)

	// IPv6 溢出
	maxV6 := netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	_, err = AddrAdd(maxV6, 1)
	assert.ErrorIs(t, err, ErrOverflow)

	// IPv6 下溢
	minV6 := netip.MustParseAddr("::")
	_, err = AddrAdd(minV6, -1)
	assert.ErrorIs(t, err, ErrOverflow)
}

func TestAddrAdd_Invalid(t *testing.T) {
	_, err := AddrAdd(netip.Addr{}, 1)
	assert.ErrorIs(t, err, ErrInvalidAddress)
}
