package xnet

import (
	"math/big"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionString(t *testing.T) {
	assert.Equal(t, "IPv4", V4.String())
	assert.Equal(t, "IPv6", V6.String())
	assert.Equal(t, "unknown", V0.String())
	assert.Equal(t, "unknown", Version(99).String())
}

func TestAddrVersion(t *testing.T) {
	assert.Equal(t, V4, AddrVersion(netip.MustParseAddr("192.168.1.1")))
	assert.Equal(t, V6, AddrVersion(netip.MustParseAddr("::1")))
	assert.Equal(t, V6, AddrVersion(netip.MustParseAddr("2001:db8::1")))

	// IPv4-mapped IPv6 地址视为 V4
	assert.Equal(t, V4, AddrVersion(netip.MustParseAddr("::ffff:192.168.1.1")))

	// 无效地址返回 V0
	assert.Equal(t, V0, AddrVersion(netip.Addr{}))
}

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
