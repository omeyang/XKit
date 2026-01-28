package xnet

import (
	"fmt"
	"net/netip"

	"go4.org/netipx"
)

// WireRange 是 IP 范围的序列化格式。
// 使用 JSON 标签 {"s":"start","e":"end"} 和 BSON 标签。
type WireRange struct {
	S string `json:"s" bson:"s"`
	E string `json:"e" bson:"e"`
}

// WireRangeFrom 从 [netipx.IPRange] 创建 WireRange。
func WireRangeFrom(r netipx.IPRange) WireRange {
	return WireRange{
		S: r.From().String(),
		E: r.To().String(),
	}
}

// WireRangeFromAddrs 从起止地址创建 WireRange。
// from 和 to 必须是有效地址且 from <= to，否则返回错误。
func WireRangeFromAddrs(from, to netip.Addr) (WireRange, error) {
	if !from.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid start address", ErrInvalidAddress)
	}
	if !to.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid end address", ErrInvalidAddress)
	}
	if AddrVersion(from) != AddrVersion(to) {
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
