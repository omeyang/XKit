package xnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/netipx"
)

func TestRangeContainsV4(t *testing.T) {
	from := netip.MustParseAddr("192.168.1.1")
	to := netip.MustParseAddr("192.168.1.100")

	tests := []struct {
		name string
		addr netip.Addr
		want bool
	}{
		{"start boundary", netip.MustParseAddr("192.168.1.1"), true},
		{"end boundary", netip.MustParseAddr("192.168.1.100"), true},
		{"middle", netip.MustParseAddr("192.168.1.50"), true},
		{"below range", netip.MustParseAddr("192.168.1.0"), false},
		{"above range", netip.MustParseAddr("192.168.1.101"), false},
		{"different subnet", netip.MustParseAddr("10.0.0.1"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RangeContainsV4(from, to, tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRangeContainsV4_IPv6Returns_False(t *testing.T) {
	from := netip.MustParseAddr("192.168.1.1")
	to := netip.MustParseAddr("192.168.1.100")
	v6Addr := netip.MustParseAddr("::1")

	assert.False(t, RangeContainsV4(from, to, v6Addr))
}

func TestRangeContainsV4_IPv6Range_ReturnsFalse(t *testing.T) {
	from := netip.MustParseAddr("::1")
	to := netip.MustParseAddr("::ff")
	addr := netip.MustParseAddr("::50")

	assert.False(t, RangeContainsV4(from, to, addr))
}

func TestRangeContainsV4_FullRange(t *testing.T) {
	from := netip.MustParseAddr("0.0.0.0")
	to := netip.MustParseAddr("255.255.255.255")

	assert.True(t, RangeContainsV4(from, to, netip.MustParseAddr("0.0.0.0")))
	assert.True(t, RangeContainsV4(from, to, netip.MustParseAddr("255.255.255.255")))
	assert.True(t, RangeContainsV4(from, to, netip.MustParseAddr("128.0.0.1")))
}

func TestIPSetFromRanges(t *testing.T) {
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("192.168.1.0"),
			netip.MustParseAddr("192.168.1.255"),
		),
	}

	set, err := IPSetFromRanges(ranges)
	require.NoError(t, err)

	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.50")))
	assert.True(t, set.Contains(netip.MustParseAddr("192.168.1.128")))
	assert.False(t, set.Contains(netip.MustParseAddr("8.8.8.8")))
}

func TestIPSetFromRanges_Empty(t *testing.T) {
	set, err := IPSetFromRanges(nil)
	require.NoError(t, err)
	assert.NotNil(t, set)
	assert.Equal(t, 0, len(set.Ranges()))

	set, err = IPSetFromRanges([]netipx.IPRange{})
	require.NoError(t, err)
	assert.NotNil(t, set)
	assert.Equal(t, 0, len(set.Ranges()))
}

func TestIPSetFromRanges_Overlapping(t *testing.T) {
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.50"),
			netip.MustParseAddr("10.0.0.150"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
	}

	set, err := IPSetFromRanges(ranges)
	require.NoError(t, err)

	// Should merge to single range
	result := set.Ranges()
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "10.0.0.1", result[0].From().String())
	assert.Equal(t, "10.0.0.150", result[0].To().String())
}

func TestMergeRanges(t *testing.T) {
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.50"),
			netip.MustParseAddr("10.0.0.150"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("192.168.1.0"),
			netip.MustParseAddr("192.168.1.255"),
		),
	}

	merged, err := MergeRanges(ranges)
	require.NoError(t, err)
	assert.Equal(t, 2, len(merged))

	// Should be sorted: 10.x before 192.x
	assert.Equal(t, "10.0.0.1", merged[0].From().String())
	assert.Equal(t, "10.0.0.150", merged[0].To().String())
	assert.Equal(t, "192.168.1.0", merged[1].From().String())
	assert.Equal(t, "192.168.1.255", merged[1].To().String())
}

func TestMergeRanges_Adjacent(t *testing.T) {
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.101"),
			netip.MustParseAddr("10.0.0.200"),
		),
	}

	merged, err := MergeRanges(ranges)
	require.NoError(t, err)
	assert.Equal(t, 1, len(merged))
	assert.Equal(t, "10.0.0.1", merged[0].From().String())
	assert.Equal(t, "10.0.0.200", merged[0].To().String())
}

func TestMergeRanges_Empty(t *testing.T) {
	merged, err := MergeRanges(nil)
	require.NoError(t, err)
	assert.Nil(t, merged)

	merged, err = MergeRanges([]netipx.IPRange{})
	require.NoError(t, err)
	assert.Nil(t, merged)
}

func TestMergeRanges_Single(t *testing.T) {
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
	}

	merged, err := MergeRanges(ranges)
	require.NoError(t, err)
	assert.Equal(t, 1, len(merged))
	assert.Equal(t, ranges[0], merged[0])
}

func TestMergeRanges_InvalidRange(t *testing.T) {
	// 包含 From > To 的无效范围
	ranges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
		netipx.IPRangeFrom(
			netip.MustParseAddr("192.168.1.100"), // From > To
			netip.MustParseAddr("192.168.1.1"),
		),
	}

	_, err := MergeRanges(ranges)
	assert.ErrorIs(t, err, ErrInvalidRange)
	assert.Contains(t, err.Error(), "range [1]")
}

func TestIPSetFromRangesStrict(t *testing.T) {
	// 有效范围
	validRanges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.1"),
			netip.MustParseAddr("10.0.0.100"),
		),
	}
	set, err := IPSetFromRangesStrict(validRanges)
	require.NoError(t, err)
	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.50")))

	// 无效范围
	invalidRanges := []netipx.IPRange{
		netipx.IPRangeFrom(
			netip.MustParseAddr("10.0.0.100"),
			netip.MustParseAddr("10.0.0.1"),
		),
	}
	_, err = IPSetFromRangesStrict(invalidRanges)
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestRangeSize(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantSize int64
	}{
		{"single IP", "192.168.1.1", "192.168.1.1", 1},
		{"/24 subnet", "192.168.1.0", "192.168.1.255", 256},
		{"/16 subnet", "192.168.0.0", "192.168.255.255", 65536},
		{"custom range", "10.0.0.1", "10.0.0.100", 100},
		{"full IPv4", "0.0.0.0", "255.255.255.255", 4294967296},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := netipx.IPRangeFrom(
				netip.MustParseAddr(tt.from),
				netip.MustParseAddr(tt.to),
			)
			size := RangeSize(r)
			require.NotNil(t, size)
			assert.Equal(t, tt.wantSize, size.Int64())
		})
	}
}

func TestRangeSize_IPv6(t *testing.T) {
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("::ff"),
	)
	size := RangeSize(r)
	require.NotNil(t, size)
	assert.Equal(t, int64(255), size.Int64())
}

func TestRangeSize_Invalid(t *testing.T) {
	// 无效范围（From > To）
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.100"),
		netip.MustParseAddr("192.168.1.1"),
	)
	size := RangeSize(r)
	assert.Nil(t, size)
}

func TestRangeSizeUint64(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantSize uint64
		wantOK   bool
	}{
		{"single IP", "192.168.1.1", "192.168.1.1", 1, true},
		{"/24 subnet", "192.168.1.0", "192.168.1.255", 256, true},
		{"custom range", "10.0.0.1", "10.0.0.100", 100, true},
		{"full IPv4", "0.0.0.0", "255.255.255.255", 4294967296, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := netipx.IPRangeFrom(
				netip.MustParseAddr(tt.from),
				netip.MustParseAddr(tt.to),
			)
			size, ok := RangeSizeUint64(r)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantSize, size)
			}
		})
	}
}

func TestRangeSizeUint64_IPv6(t *testing.T) {
	// IPv6 范围不支持 uint64 优化
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("::ff"),
	)
	_, ok := RangeSizeUint64(r)
	assert.False(t, ok)
}

func TestRangeSizeUint64_Invalid(t *testing.T) {
	// 无效范围
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.100"),
		netip.MustParseAddr("192.168.1.1"),
	)
	_, ok := RangeSizeUint64(r)
	assert.False(t, ok)
}

func TestRangeToPrefix(t *testing.T) {
	tests := []struct {
		name       string
		from       string
		to         string
		wantPrefix string
		wantOK     bool
	}{
		{
			name:       "exact /24",
			from:       "192.168.1.0",
			to:         "192.168.1.255",
			wantPrefix: "192.168.1.0/24",
			wantOK:     true,
		},
		{
			name:       "exact /32 single IP",
			from:       "192.168.1.1",
			to:         "192.168.1.1",
			wantPrefix: "192.168.1.1/32",
			wantOK:     true,
		},
		{
			name:       "exact /16",
			from:       "192.168.0.0",
			to:         "192.168.255.255",
			wantPrefix: "192.168.0.0/16",
			wantOK:     true,
		},
		{
			name:       "exact /0 full IPv4",
			from:       "0.0.0.0",
			to:         "255.255.255.255",
			wantPrefix: "0.0.0.0/0",
			wantOK:     true,
		},
		{
			name:   "not a CIDR block",
			from:   "192.168.1.1",
			to:     "192.168.1.100",
			wantOK: false,
		},
		{
			name:   "not aligned",
			from:   "192.168.1.1",
			to:     "192.168.1.254",
			wantOK: false,
		},
		{
			name:       "IPv6 /128",
			from:       "2001:db8::1",
			to:         "2001:db8::1",
			wantPrefix: "2001:db8::1/128",
			wantOK:     true,
		},
		{
			name:       "IPv6 /64",
			from:       "2001:db8::",
			to:         "2001:db8::ffff:ffff:ffff:ffff",
			wantPrefix: "2001:db8::/64",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := netipx.IPRangeFrom(
				netip.MustParseAddr(tt.from),
				netip.MustParseAddr(tt.to),
			)
			prefix, ok := RangeToPrefix(r)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantPrefix, prefix.String())
			}
		})
	}
}

func TestRangeToPrefix_Invalid(t *testing.T) {
	// 无效范围
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.100"),
		netip.MustParseAddr("192.168.1.1"),
	)
	_, ok := RangeToPrefix(r)
	assert.False(t, ok)
}

func TestRangeToPrefixes(t *testing.T) {
	tests := []struct {
		name         string
		from         string
		to           string
		wantExact    int // -1 表示不检查精确数量
		wantMinCount int // 最小数量
	}{
		{
			name:      "exact /24 → 1 prefix",
			from:      "192.168.1.0",
			to:        "192.168.1.255",
			wantExact: 1,
		},
		{
			name:      "single IP → 1 prefix",
			from:      "192.168.1.1",
			to:        "192.168.1.1",
			wantExact: 1,
		},
		{
			name:         "non-CIDR range → multiple prefixes",
			from:         "192.168.1.1",
			to:           "192.168.1.100",
			wantExact:    -1, // 不检查精确数量，因为可能随算法变化
			wantMinCount: 2,  // 至少需要 2 个 CIDR 块
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := netipx.IPRangeFrom(
				netip.MustParseAddr(tt.from),
				netip.MustParseAddr(tt.to),
			)
			prefixes := RangeToPrefixes(r)

			if tt.wantExact >= 0 {
				assert.Equal(t, tt.wantExact, len(prefixes))
			} else {
				assert.GreaterOrEqual(t, len(prefixes), tt.wantMinCount)
			}

			// 验证所有前缀的并集等于原范围
			var sb netipx.IPSetBuilder
			for _, p := range prefixes {
				sb.AddPrefix(p)
			}
			set, err := sb.IPSet()
			require.NoError(t, err)

			// 原范围的起止地址应该在集合中
			assert.True(t, set.Contains(r.From()))
			assert.True(t, set.Contains(r.To()))

			// 验证集合的范围与原范围完全一致
			resultRanges := set.Ranges()
			require.Equal(t, 1, len(resultRanges))
			assert.Equal(t, r.From(), resultRanges[0].From())
			assert.Equal(t, r.To(), resultRanges[0].To())
		})
	}
}

func TestRangeToPrefixes_Invalid(t *testing.T) {
	// 无效范围返回 nil
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.100"),
		netip.MustParseAddr("192.168.1.1"),
	)
	prefixes := RangeToPrefixes(r)
	assert.Nil(t, prefixes)
}
