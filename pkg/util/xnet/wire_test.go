package xnet

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go4.org/netipx"
)

func TestWireRangeFrom(t *testing.T) {
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("192.168.1.100"),
	)
	w := WireRangeFrom(r)
	assert.Equal(t, "192.168.1.1", w.S)
	assert.Equal(t, "192.168.1.100", w.E)
}

func TestWireRangeFromChecked(t *testing.T) {
	// 有效范围
	validRange := netipx.IPRangeFrom(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.100"),
	)
	w, err := WireRangeFromChecked(validRange)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1", w.S)
	assert.Equal(t, "10.0.0.100", w.E)

	// 无效范围 (From > To)
	invalidRange := netipx.IPRangeFrom(
		netip.MustParseAddr("10.0.0.100"),
		netip.MustParseAddr("10.0.0.1"),
	)
	_, err = WireRangeFromChecked(invalidRange)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// 零值范围
	var zeroRange netipx.IPRange
	_, err = WireRangeFromChecked(zeroRange)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// IPv6 有效范围
	ipv6Range := netipx.IPRangeFrom(
		netip.MustParseAddr("2001:db8::1"),
		netip.MustParseAddr("2001:db8::ff"),
	)
	w, err = WireRangeFromChecked(ipv6Range)
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::1", w.S)
	assert.Equal(t, "2001:db8::ff", w.E)
}

func TestWireRangeFromAddrs(t *testing.T) {
	w, err := WireRangeFromAddrs(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.100"),
	)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1", w.S)
	assert.Equal(t, "10.0.0.100", w.E)

	// Invalid: zero-value addr
	_, err = WireRangeFromAddrs(netip.Addr{}, netip.MustParseAddr("10.0.0.1"))
	assert.ErrorIs(t, err, ErrInvalidAddress)

	_, err = WireRangeFromAddrs(netip.MustParseAddr("10.0.0.1"), netip.Addr{})
	assert.ErrorIs(t, err, ErrInvalidAddress)

	// Invalid: from > to
	_, err = WireRangeFromAddrs(
		netip.MustParseAddr("10.0.0.100"),
		netip.MustParseAddr("10.0.0.1"),
	)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// Invalid: mixed address families (IPv4 start, IPv6 end)
	_, err = WireRangeFromAddrs(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("2001:db8::1"),
	)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// Invalid: mixed address families (IPv6 start, IPv4 end)
	_, err = WireRangeFromAddrs(
		netip.MustParseAddr("2001:db8::1"),
		netip.MustParseAddr("10.0.0.1"),
	)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// Invalid: pure IPv4 mixed with IPv4-mapped IPv6
	_, err = WireRangeFromAddrs(
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("::ffff:192.168.1.100"),
	)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// Invalid: IPv4-mapped IPv6 mixed with pure IPv4
	_, err = WireRangeFromAddrs(
		netip.MustParseAddr("::ffff:192.168.1.1"),
		netip.MustParseAddr("192.168.1.100"),
	)
	assert.ErrorIs(t, err, ErrInvalidRange)

	// Valid: both IPv4-mapped IPv6
	w, err = WireRangeFromAddrs(
		netip.MustParseAddr("::ffff:192.168.1.1"),
		netip.MustParseAddr("::ffff:192.168.1.100"),
	)
	require.NoError(t, err)
	assert.Equal(t, "::ffff:192.168.1.1", w.S)
	assert.Equal(t, "::ffff:192.168.1.100", w.E)
}

func TestWireRangeFromAddrs_MixedIPv4AndMapped(t *testing.T) {
	// 测试纯 IPv4 与 IPv4-mapped IPv6 被视为不同族
	v4 := netip.MustParseAddr("192.168.1.1")
	v4in6 := netip.MustParseAddr("::ffff:192.168.1.100")

	// 验证两者的 AddrVersion 相同（都是 V4）
	assert.Equal(t, V4, AddrVersion(v4))
	assert.Equal(t, V4, AddrVersion(v4in6))

	// 但 WireRangeFromAddrs 应该拒绝混合
	_, err := WireRangeFromAddrs(v4, v4in6)
	assert.ErrorIs(t, err, ErrInvalidRange)
	assert.Contains(t, err.Error(), "mixed address families")

	_, err = WireRangeFromAddrs(v4in6, v4)
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestWireRangeToIPRange(t *testing.T) {
	w := WireRange{S: "192.168.1.1", E: "192.168.1.100"}
	r, err := w.ToIPRange()
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", r.From().String())
	assert.Equal(t, "192.168.1.100", r.To().String())

	// Invalid start
	w = WireRange{S: "invalid", E: "192.168.1.1"}
	_, err = w.ToIPRange()
	assert.ErrorIs(t, err, ErrInvalidAddress)

	// Invalid end
	w = WireRange{S: "192.168.1.1", E: "invalid"}
	_, err = w.ToIPRange()
	assert.ErrorIs(t, err, ErrInvalidAddress)

	// Inverted range (start > end → invalid IPRange)
	w = WireRange{S: "192.168.1.100", E: "192.168.1.1"}
	_, err = w.ToIPRange()
	assert.ErrorIs(t, err, ErrInvalidRange)
}

func TestWireRangeString(t *testing.T) {
	w := WireRange{S: "10.0.0.1", E: "10.0.0.100"}
	assert.Equal(t, "10.0.0.1-10.0.0.100", w.String())

	// Single IP
	w = WireRange{S: "192.168.1.1", E: "192.168.1.1"}
	assert.Equal(t, "192.168.1.1", w.String())
}

func TestWireRangeJSON(t *testing.T) {
	w := WireRange{S: "192.168.1.1", E: "192.168.1.100"}

	// Marshal
	data, err := json.Marshal(w)
	require.NoError(t, err)
	assert.JSONEq(t, `{"s":"192.168.1.1","e":"192.168.1.100"}`, string(data))

	// Unmarshal
	var w2 WireRange
	err = json.Unmarshal(data, &w2)
	require.NoError(t, err)
	assert.Equal(t, w, w2)

	// Round-trip to IPRange
	r, err := w2.ToIPRange()
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", r.From().String())
	assert.Equal(t, "192.168.1.100", r.To().String())
}

func TestWireRangeBSON(t *testing.T) {
	w := WireRange{S: "192.168.1.1", E: "192.168.1.100"}

	// Marshal
	data, err := bson.Marshal(w)
	require.NoError(t, err)

	// Unmarshal
	var w2 WireRange
	err = bson.Unmarshal(data, &w2)
	require.NoError(t, err)
	assert.Equal(t, w, w2)

	// Round-trip to IPRange
	r, err := w2.ToIPRange()
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", r.From().String())
	assert.Equal(t, "192.168.1.100", r.To().String())
}

func TestWireRangesFromSetNil(t *testing.T) {
	wrs := WireRangesFromSet(nil)
	assert.Nil(t, wrs)
}

func TestWireRangesFromSet(t *testing.T) {
	set, err := ParseRanges([]string{
		"10.0.0.1-10.0.0.100",
		"192.168.1.0/24",
	})
	require.NoError(t, err)

	wrs := WireRangesFromSet(set)
	assert.Equal(t, 2, len(wrs))
	// IPSet 排序：10.x 在前
	assert.Equal(t, "10.0.0.1", wrs[0].S)
	assert.Equal(t, "10.0.0.100", wrs[0].E)
	assert.Equal(t, "192.168.1.0", wrs[1].S)
	assert.Equal(t, "192.168.1.255", wrs[1].E)
}

func TestWireRangesToSet(t *testing.T) {
	wrs := []WireRange{
		{S: "10.0.0.1", E: "10.0.0.100"},
		{S: "192.168.1.0", E: "192.168.1.255"},
	}
	set, err := WireRangesToSet(wrs)
	require.NoError(t, err)

	assert.True(t, set.Contains(netip.MustParseAddr("10.0.0.50")))
	assert.True(t, set.Contains(netip.MustParseAddr("192.168.1.128")))
	assert.False(t, set.Contains(netip.MustParseAddr("8.8.8.8")))

	// Invalid wire range
	wrs = []WireRange{{S: "invalid", E: "10.0.0.1"}}
	_, err = WireRangesToSet(wrs)
	assert.ErrorIs(t, err, ErrInvalidAddress)
}

func TestWireRangeRoundTrip(t *testing.T) {
	// IPRange → WireRange → IPRange
	r := netipx.IPRangeFrom(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.100"),
	)
	w := WireRangeFrom(r)
	r2, err := w.ToIPRange()
	require.NoError(t, err)
	assert.Equal(t, r, r2)
}

func TestWireRangesSetRoundTrip(t *testing.T) {
	// IPSet → []WireRange → IPSet
	set, err := ParseRanges([]string{
		"10.0.0.1-10.0.0.100",
		"192.168.1.0/24",
	})
	require.NoError(t, err)

	wrs := WireRangesFromSet(set)
	set2, err := WireRangesToSet(wrs)
	require.NoError(t, err)

	// 验证两个 set 内容一致
	ranges1 := set.Ranges()
	ranges2 := set2.Ranges()
	require.Equal(t, len(ranges1), len(ranges2))
	for i := range ranges1 {
		assert.Equal(t, ranges1[i], ranges2[i])
	}
}
