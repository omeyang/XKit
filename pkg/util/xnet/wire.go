package xnet

import (
	"fmt"
	"net/netip"

	"go4.org/netipx"
)

// WireRange 是 IP 范围的序列化格式。
// 使用 JSON/BSON/YAML 标签 {"s":"start","e":"end"}。
type WireRange struct {
	S string `json:"s" bson:"s" yaml:"s"`
	E string `json:"e" bson:"e" yaml:"e"`
}

// WireRangeFrom 从 [netipx.IPRange] 创建 WireRange。
//
// 注意：此函数不校验 r 的有效性，无效范围会静默转换为可能不正确的字符串。
// 如需校验，请使用 [WireRangeFromChecked] 或 [WireRangeFromAddrs]。
func WireRangeFrom(r netipx.IPRange) WireRange {
	return WireRange{
		S: r.From().String(),
		E: r.To().String(),
	}
}

// WireRangeFromChecked 从 [netipx.IPRange] 创建 WireRange，带有效性校验。
// 如果 r 无效（From > To 或包含无效地址），返回错误。
func WireRangeFromChecked(r netipx.IPRange) (WireRange, error) {
	if !r.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid IPRange", ErrInvalidRange)
	}
	return WireRangeFrom(r), nil
}

// WireRangeFromAddrs 从起止地址创建 WireRange。
// from 和 to 必须是有效地址且 from <= to，否则返回错误。
// 注意：纯 IPv4 地址和 IPv4-mapped IPv6 地址被视为不同族，不能混合使用。
func WireRangeFromAddrs(from, to netip.Addr) (WireRange, error) {
	if !from.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid start address", ErrInvalidAddress)
	}
	if !to.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid end address", ErrInvalidAddress)
	}
	if !sameWireFamily(from, to) {
		return WireRange{}, fmt.Errorf("%w: mixed address families: %s and %s", ErrInvalidRange, from, to)
	}
	if from.Compare(to) > 0 {
		return WireRange{}, fmt.Errorf("%w: start %s > end %s", ErrInvalidRange, from, to)
	}
	return WireRange{
		S: from.String(),
		E: to.String(),
	}, nil
}

// sameWireFamily 检查两个地址是否属于同一"线路族"。
// 纯 IPv4 (Is4) 和 IPv4-mapped IPv6 (Is4In6) 被视为不同族。
// 这避免了序列化/反序列化时的混淆问题。
func sameWireFamily(a, b netip.Addr) bool {
	// 检查两者是否都是纯 IPv4
	if a.Is4() && b.Is4() {
		return true
	}
	// 检查两者是否都是 IPv4-mapped IPv6
	if a.Is4In6() && b.Is4In6() {
		return true
	}
	// 检查两者是否都是纯 IPv6（非 IPv4-mapped）
	if a.Is6() && !a.Is4In6() && b.Is6() && !b.Is4In6() {
		return true
	}
	return false
}

// ToIPRange 将 WireRange 转换为 [netipx.IPRange]。
func (w WireRange) ToIPRange() (netipx.IPRange, error) {
	start, err := netip.ParseAddr(w.S)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid start address: %s", ErrInvalidAddress, w.S)
	}
	end, err := netip.ParseAddr(w.E)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid end address: %s", ErrInvalidAddress, w.E)
	}
	r := netipx.IPRangeFrom(start, end)
	if !r.IsValid() {
		return netipx.IPRange{}, fmt.Errorf("%w: %s-%s", ErrInvalidRange, w.S, w.E)
	}
	return r, nil
}

// String 返回 WireRange 的字符串表示："start-end"。
// 如果起止相同则只返回单个 IP。
func (w WireRange) String() string {
	if w.S == w.E {
		return w.S
	}
	return w.S + "-" + w.E
}

// WireRangesFromSet 将 [*netipx.IPSet] 转换为 WireRange 切片。
// set 为 nil 时返回 nil。
func WireRangesFromSet(set *netipx.IPSet) []WireRange {
	if set == nil {
		return nil
	}
	ranges := set.Ranges()
	wrs := make([]WireRange, len(ranges))
	for i, r := range ranges {
		wrs[i] = WireRangeFrom(r)
	}
	return wrs
}

// WireRangesToSet 将 WireRange 切片转换为 [*netipx.IPSet]。
func WireRangesToSet(wrs []WireRange) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for i, w := range wrs {
		r, err := w.ToIPRange()
		if err != nil {
			return nil, fmt.Errorf("wire range [%d] %q: %w", i, w.String(), err)
		}
		b.AddRange(r)
	}
	set, err := b.IPSet()
	if err != nil {
		return nil, fmt.Errorf("build IPSet: %w", err)
	}
	return set, nil
}
