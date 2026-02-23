package xnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatFullIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "192.168.001.001"},
		{"10.0.0.1", "010.000.000.001"},
		{"0.0.0.0", "000.000.000.000"},
		{"255.255.255.255", "255.255.255.255"},
		{"1.2.3.4", "001.002.003.004"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := FormatFullIP(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	// IPv6
	got, err := FormatFullIP("::1")
	require.NoError(t, err)
	assert.Equal(t, "00000000000000000000000000000001", got)
	assert.Len(t, got, 32)

	// Invalid
	_, err = FormatFullIP("invalid")
	assert.ErrorIs(t, err, ErrInvalidAddress)
}

func TestFormatFullIPAddr(t *testing.T) {
	// IPv4
	addr := netip.MustParseAddr("192.168.1.1")
	assert.Equal(t, "192.168.001.001", FormatFullIPAddr(addr))

	// IPv6
	addr = netip.MustParseAddr("::1")
	assert.Equal(t, "00000000000000000000000000000001", FormatFullIPAddr(addr))

	// Invalid
	assert.Equal(t, "", FormatFullIPAddr(netip.Addr{}))
}

func TestParseFullIP(t *testing.T) {
	// IPv4 padded format
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.001.001", "192.168.1.1"},
		{"010.000.000.001", "10.0.0.1"},
		{"000.000.000.000", "0.0.0.0"},
		{"255.255.255.255", "255.255.255.255"},
		{"1.2.3.4", "1.2.3.4"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			addr, err := ParseFullIP(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, addr.String())
		})
	}

	// IPv6 full hex
	addr, err := ParseFullIP("00000000000000000000000000000001")
	require.NoError(t, err)
	assert.Equal(t, "::1", addr.String())

	// Invalid
	_, err = ParseFullIP("invalid")
	assert.ErrorIs(t, err, ErrInvalidAddress)

	// Malformed dotted inputs
	malformed := []string{
		"1..3.4",        // 空段
		"1.2.3.4.5",     // 5 段
		".1.2.3",        // 前导点
		"1.2.3.",        // 尾部点（3 段 + 空串）
		"1.2.3.+4",      // 非数字
		"1.2.3.999",     // 超范围
		"abc.def.ghi.j", // 非数字字符
	}
	for _, s := range malformed {
		_, err = ParseFullIP(s)
		assert.ErrorIs(t, err, ErrInvalidAddress, "ParseFullIP(%q) should wrap ErrInvalidAddress", s)
	}
}

func TestParseFullIP_IPv4MappedIPv6(t *testing.T) {
	// IPv4-mapped IPv6 地址应通过标准解析器回退正确解析
	addr, err := ParseFullIP("::ffff:192.168.1.1")
	require.NoError(t, err)
	assert.True(t, addr.Is4In6())
	assert.Equal(t, "::ffff:192.168.1.1", addr.String())
}

func TestFullIPRoundTrip(t *testing.T) {
	inputs := []string{
		"192.168.1.1",
		"10.0.0.1",
		"0.0.0.0",
		"255.255.255.255",
		"::1",
		"2001:db8::1",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			orig := netip.MustParseAddr(input)
			full := FormatFullIPAddr(orig)
			restored, err := ParseFullIP(full)
			require.NoError(t, err)
			assert.Equal(t, 0, orig.Compare(restored), "round-trip failed: %s → %s", input, full)
		})
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "192.168.1.1"},
		{"::1", "::1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeIP(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := NormalizeIP("invalid")
	assert.ErrorIs(t, err, ErrInvalidAddress)
}

func TestIsValidIP(t *testing.T) {
	assert.True(t, IsValidIP("192.168.1.1"))
	assert.True(t, IsValidIP("::1"))
	assert.True(t, IsValidIP("2001:db8::1"))
	assert.False(t, IsValidIP(""))
	assert.False(t, IsValidIP("invalid"))
	assert.False(t, IsValidIP("256.1.1.1"))
}

func TestFullIPKnownValues(t *testing.T) {
	// 验证 FormatFullIPAddr 和 ParseFullIP 对常见地址的正确性
	compat := map[string]string{
		"192.168.1.1":     "192.168.001.001",
		"10.0.0.1":        "010.000.000.001",
		"255.255.255.255": "255.255.255.255",
		"0.0.0.0":         "000.000.000.000",
	}
	for ip, wantFull := range compat {
		addr := netip.MustParseAddr(ip)
		assert.Equal(t, wantFull, FormatFullIPAddr(addr), "FormatFullIPAddr mismatch for %s", ip)

		restored, err := ParseFullIP(wantFull)
		require.NoError(t, err)
		assert.Equal(t, ip, restored.String(), "ParseFullIP mismatch for %s", wantFull)
	}
}
