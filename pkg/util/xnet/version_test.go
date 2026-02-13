package xnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
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
