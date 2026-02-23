package xnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart string
		wantEnd   string
		wantErr   bool
	}{
		{
			name:      "single IP",
			input:     "192.168.1.1",
			wantStart: "192.168.1.1",
			wantEnd:   "192.168.1.1",
		},
		{
			name:      "CIDR /24",
			input:     "192.168.1.0/24",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "CIDR /32",
			input:     "10.0.0.1/32",
			wantStart: "10.0.0.1",
			wantEnd:   "10.0.0.1",
		},
		{
			name:      "CIDR /16",
			input:     "172.16.0.0/16",
			wantStart: "172.16.0.0",
			wantEnd:   "172.16.255.255",
		},
		{
			name:      "mask notation",
			input:     "192.168.1.0/255.255.255.0",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "explicit range",
			input:     "10.0.0.1-10.0.0.100",
			wantStart: "10.0.0.1",
			wantEnd:   "10.0.0.100",
		},
		{
			name:      "IPv6 CIDR",
			input:     "2001:db8::/32",
			wantStart: "2001:db8::",
			wantEnd:   "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		},
		{
			name:    "invalid",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "invalid range start",
			input:   "invalid-10.0.0.1",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			input:   "192.168.1.0/99",
			wantErr: true,
		},
		{
			name:    "non-contiguous mask",
			input:   "192.168.1.0/255.0.255.0",
			wantErr: true,
		},
		{
			name:      "full mask (/32 equivalent)",
			input:     "192.168.1.1/255.255.255.255",
			wantStart: "192.168.1.1",
			wantEnd:   "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidRange)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, r.From().String())
			assert.Equal(t, tt.wantEnd, r.To().String())
		})
	}
}

func TestParseRangeContains(t *testing.T) {
	r, err := ParseRange("192.168.1.0/24")
	require.NoError(t, err)

	assert.True(t, r.Contains(netip.MustParseAddr("192.168.1.0")))
	assert.True(t, r.Contains(netip.MustParseAddr("192.168.1.1")))
	assert.True(t, r.Contains(netip.MustParseAddr("192.168.1.255")))
	assert.True(t, r.Contains(netip.MustParseAddr("192.168.1.128")))
	assert.False(t, r.Contains(netip.MustParseAddr("192.168.2.0")))
	assert.False(t, r.Contains(netip.MustParseAddr("192.168.0.255")))
	assert.False(t, r.Contains(netip.MustParseAddr("10.0.0.1")))
}

func TestParseRanges(t *testing.T) {
	set, err := ParseRanges([]string{
		"10.0.0.1-10.0.0.100",
		"192.168.1.0/24",
		"172.16.0.1",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, len(set.Ranges()))

	// Contains 检查
	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.50")))
	assert.True(t, set.Contains(netip.MustParseAddr("192.168.1.128")))
	assert.True(t, set.Contains(netip.MustParseAddr("172.16.0.1")))
	assert.False(t, set.Contains(netip.MustParseAddr("8.8.8.8")))

	// Invalid
	_, err = ParseRanges([]string{"invalid"})
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestParseRangesMergesOverlapping(t *testing.T) {
	// ParseRanges 返回的 IPSet 自动合并重叠/相邻范围
	set, err := ParseRanges([]string{
		"10.0.0.50-10.0.0.150",
		"10.0.0.1-10.0.0.100",
		"10.0.0.200-10.0.0.255",
		"10.0.0.151-10.0.0.199", // 与邻居相邻
	})
	require.NoError(t, err)

	ranges := set.Ranges()
	assert.Equal(t, 1, len(ranges), "all ranges should merge into one")
	assert.Equal(t, "10.0.0.1", ranges[0].From().String())
	assert.Equal(t, "10.0.0.255", ranges[0].To().String())
}

func TestParseRangesDisjoint(t *testing.T) {
	set, err := ParseRanges([]string{
		"192.168.1.0-192.168.1.100",
		"10.0.0.1-10.0.0.50",
		"172.16.0.1-172.16.0.10",
	})
	require.NoError(t, err)

	ranges := set.Ranges()
	assert.Equal(t, 3, len(ranges))

	// IPSet 内部自动排序
	assert.Equal(t, "10.0.0.1", ranges[0].From().String())
	assert.Equal(t, "172.16.0.1", ranges[1].From().String())
	assert.Equal(t, "192.168.1.0", ranges[2].From().String())
}

func TestParseRangesEmpty(t *testing.T) {
	// nil
	set, err := ParseRanges(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(set.Ranges()))

	// 空切片
	set, err = ParseRanges([]string{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(set.Ranges()))
}

func TestParseRangeWhitespace(t *testing.T) {
	// 带前后空白的输入应自动 trim
	r, err := ParseRange("  192.168.1.0/24  ")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.0", r.From().String())
	assert.Equal(t, "192.168.1.255", r.To().String())

	r, err = ParseRange(" 10.0.0.1-10.0.0.100 ")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1", r.From().String())
	assert.Equal(t, "10.0.0.100", r.To().String())

	r, err = ParseRange("\t192.168.1.1\n")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", r.From().String())
}

func TestParseRangeWhitespaceAroundSlash(t *testing.T) {
	// 与 "-" 分隔符一致，"/" 分隔符两侧的空白也应被处理
	tests := []struct {
		name      string
		input     string
		wantStart string
		wantEnd   string
	}{
		{
			name:      "CIDR 斜杠前后有空白",
			input:     "192.168.1.0 / 24",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "掩码格式斜杠前后有空白",
			input:     "192.168.1.0 / 255.255.255.0",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "CIDR 斜杠前有空白",
			input:     "10.0.0.0 /8",
			wantStart: "10.0.0.0",
			wantEnd:   "10.255.255.255",
		},
		{
			name:      "CIDR 斜杠后有空白",
			input:     "172.16.0.0/ 16",
			wantStart: "172.16.0.0",
			wantEnd:   "172.16.255.255",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, r.From().String())
			assert.Equal(t, tt.wantEnd, r.To().String())
		})
	}
}

func TestParseRangeIPv6Range(t *testing.T) {
	r, err := ParseRange("::1-::ff")
	require.NoError(t, err)
	assert.Equal(t, "::1", r.From().String())
	assert.Equal(t, "::ff", r.To().String())
}

func TestParseRangesContainsBoundary(t *testing.T) {
	set, err := ParseRanges([]string{
		"10.0.0.1-10.0.0.100",
		"192.168.1.0/24",
		"172.16.0.1-172.16.0.10",
	})
	require.NoError(t, err)

	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.1")))   // 边界
	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.100"))) // 边界
	assert.False(t, set.Contains(netip.MustParseAddr("10.0.0.101")))
	assert.False(t, set.Contains(netip.MustParseAddr("192.168.2.0")))
	assert.False(t, set.Contains(netip.MustParseAddr("8.8.8.8")))
}

func TestParseRangeInvalidRangeEnd(t *testing.T) {
	_, err := ParseRange("10.0.0.1-invalid")
	assert.ErrorIs(t, err, ErrInvalidRange)
	assert.Contains(t, err.Error(), "invalid range end")
}

func TestParseRangeInvertedRange(t *testing.T) {
	// start > end
	_, err := ParseRange("10.0.0.100-10.0.0.1")
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestParseRangeBothSidesInvalid(t *testing.T) {
	// 两侧都不是合法 IP 地址，回退到 CIDR/单 IP 分支后仍然失败
	_, err := ParseRange("abc-def")
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestParseRangeWithMaskErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		errIs error
	}{
		{
			name:  "invalid address in mask notation",
			input: "invalid/255.255.255.0",
			errIs: ErrInvalidRange,
		},
		{
			name:  "invalid mask",
			input: "192.168.1.0/invalid",
			errIs: ErrInvalidRange,
		},
		{
			name:  "IPv6 with mask notation",
			input: "2001:db8::1/ffff:ffff::",
			errIs: ErrInvalidRange,
		},
		{
			name:  "IPv6 address with IPv4 mask",
			input: "2001:db8::1/255.255.255.0",
			errIs: ErrInvalidRange,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRange(tt.input)
			assert.ErrorIs(t, err, tt.errIs)
		})
	}
}

func TestParseRangeIPv4MappedIPv6(t *testing.T) {
	// IPv4-mapped IPv6 地址应该能正确解析
	r, err := ParseRange("::ffff:192.168.1.0/255.255.255.0")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.0", r.From().String())
	assert.Equal(t, "192.168.1.255", r.To().String())
}

func TestParseRangeMixedAddressFamilies(t *testing.T) {
	// IPv4 范围开始，IPv6 范围结束
	_, err := ParseRange("192.168.1.1-2001:db8::1")
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestParseRangeIPv6ZoneRejected(t *testing.T) {
	// 设计决策: 拒绝 IPv6 zone 地址（如 fe80::1%eth0），因为 netipx.IPRange/IPSet
	// 会静默丢弃 zone 信息，导致后续规则匹配失败。
	tests := []struct {
		name  string
		input string
	}{
		{"zone without dash", "fe80::1%eth0"},
		{"zone with single dash", "fe80::1%eth-0"},
		{"zone with multiple dashes", "fe80::1%br-lan-0"},
		{"zone with garbage suffix", "fe80::1%eth-0-garbage"},
		{"zone with multi-segment dash", "fe80::1%br-lan-0-test"},
		{"zone in CIDR notation", "fe80::1%eth0/64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRange(tt.input)
			assert.ErrorIs(t, err, ErrInvalidRange)
			assert.Contains(t, err.Error(), "zone")
		})
	}
}

func TestParseRangeIPv6ZoneInvalidEnd(t *testing.T) {
	// start 有效、end 无效、且整体也无法解析为单地址 → 应返回错误
	_, err := ParseRange("fe80::1-not_an_ip")
	assert.ErrorIs(t, err, ErrInvalidRange)
	assert.Contains(t, err.Error(), "invalid range end")
}

func TestParseRangeIPv4MappedMask(t *testing.T) {
	// IPv4-mapped IPv6 掩码也应该能工作
	r, err := ParseRange("192.168.1.0/::ffff:255.255.255.0")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.0", r.From().String())
	assert.Equal(t, "192.168.1.255", r.To().String())
}

func TestParseRangeIPv4MappedCIDR(t *testing.T) {
	// IPv4-mapped IPv6 + CIDR 应统一转为纯 IPv4 范围（与掩码路径行为一致）。
	tests := []struct {
		name      string
		input     string
		wantStart string
		wantEnd   string
		wantErr   bool
	}{
		{
			name:      "mapped /120 等同 IPv4 /24",
			input:     "::ffff:192.168.1.0/120",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "mapped /128 等同 IPv4 /32",
			input:     "::ffff:10.0.0.1/128",
			wantStart: "10.0.0.1",
			wantEnd:   "10.0.0.1",
		},
		{
			name:      "mapped /96 等同 IPv4 /0 全范围",
			input:     "::ffff:0.0.0.0/96",
			wantStart: "0.0.0.0",
			wantEnd:   "255.255.255.255",
		},
		{
			name:    "mapped /24 小于 96 应拒绝",
			input:   "::ffff:192.168.1.0/24",
			wantErr: true,
		},
		{
			name:    "mapped /64 小于 96 应拒绝",
			input:   "::ffff:10.0.0.0/64",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidRange)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, r.From().String())
			assert.Equal(t, tt.wantEnd, r.To().String())
		})
	}
}

func TestParseRangeMappedNormalizationConsistency(t *testing.T) {
	// FG-M1/FG-L2: 验证所有 ParseRange 语法对 IPv4-mapped IPv6 的归一化行为一致。
	// 四种语法应产生相同的纯 IPv4 范围，不保留 IPv4-mapped IPv6 形式。
	tests := []struct {
		name      string
		input     string
		wantStart string
		wantEnd   string
	}{
		{
			name:      "单 IP: mapped 归一化为纯 IPv4",
			input:     "::ffff:192.168.1.1",
			wantStart: "192.168.1.1",
			wantEnd:   "192.168.1.1",
		},
		{
			name:      "CIDR: mapped /128 归一化为纯 IPv4",
			input:     "::ffff:192.168.1.1/128",
			wantStart: "192.168.1.1",
			wantEnd:   "192.168.1.1",
		},
		{
			name:      "掩码: mapped 地址归一化为纯 IPv4",
			input:     "::ffff:192.168.1.0/255.255.255.0",
			wantStart: "192.168.1.0",
			wantEnd:   "192.168.1.255",
		},
		{
			name:      "显式范围: mapped 地址归一化为纯 IPv4",
			input:     "::ffff:192.168.1.1-::ffff:192.168.1.100",
			wantStart: "192.168.1.1",
			wantEnd:   "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, r.From().String())
			assert.Equal(t, tt.wantEnd, r.To().String())

			// 验证结果与纯 IPv4 输入一致
			pureV4, err := ParseRange(tt.wantStart + "-" + tt.wantEnd)
			require.NoError(t, err)
			assert.Equal(t, pureV4.From(), r.From(), "mapped 和纯 IPv4 应产生相同 From")
			assert.Equal(t, pureV4.To(), r.To(), "mapped 和纯 IPv4 应产生相同 To")
		})
	}
}

func TestParseRangesErrorIndex(t *testing.T) {
	// ParseRanges 错误信息应包含元素索引
	_, err := ParseRanges([]string{"10.0.0.1", "invalid", "192.168.1.0/24"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[1]")
}
