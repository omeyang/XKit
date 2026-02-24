package xnet

import (
	"fmt"
	"net/netip"

	"go4.org/netipx"
)

// WireRange 是 IP 范围的序列化格式。
// 使用 JSON/BSON/YAML 标签 {"start":"...","end":"..."}。
type WireRange struct {
	Start string `json:"start" bson:"start" yaml:"start"`
	End   string `json:"end" bson:"end" yaml:"end"`
}

// WireRangeFrom 从 [netipx.IPRange] 创建 WireRange，带有效性校验。
// 如果 r 无效（From > To 或包含无效地址），返回错误。
//
// 如需跳过校验（例如处理来自 [*netipx.IPSet].Ranges() 的已知有效范围），
// 请使用 [WireRangeFromUnchecked]。
func WireRangeFrom(r netipx.IPRange) (WireRange, error) {
	if !r.IsValid() {
		return WireRange{}, fmt.Errorf("%w: invalid IPRange", ErrInvalidRange)
	}
	return WireRangeFromUnchecked(r), nil
}

// WireRangeFromUnchecked 从 [netipx.IPRange] 创建 WireRange，不校验有效性。
//
// 注意：此函数不校验 r 的有效性，无效范围会静默转换为可能不正确的字符串。
// 仅当调用方已确保 r 有效时使用（如从 [*netipx.IPSet].Ranges() 获取的范围）。
// 一般场景请使用 [WireRangeFrom] 或 [WireRangeFromAddrs]。
func WireRangeFromUnchecked(r netipx.IPRange) WireRange {
	return WireRange{
		Start: r.From().String(),
		End:   r.To().String(),
	}
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
		Start: from.String(),
		End:   to.String(),
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
	start, err := netip.ParseAddr(w.Start)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid start address: %s", ErrInvalidAddress, w.Start)
	}
	if start.Zone() != "" {
		return netipx.IPRange{}, fmt.Errorf("%w: IPv6 zone ID is not supported: %s", ErrInvalidAddress, w.Start)
	}
	end, err := netip.ParseAddr(w.End)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid end address: %s", ErrInvalidAddress, w.End)
	}
	if end.Zone() != "" {
		return netipx.IPRange{}, fmt.Errorf("%w: IPv6 zone ID is not supported: %s", ErrInvalidAddress, w.End)
	}
	r := netipx.IPRangeFrom(start, end)
	if !r.IsValid() {
		return netipx.IPRange{}, fmt.Errorf("%w: %s-%s", ErrInvalidRange, w.Start, w.End)
	}
	return r, nil
}

// IsZero 报告 w 是否为零值。
// 零值 WireRange 的 Start 和 End 都是空字符串。
func (w WireRange) IsZero() bool {
	return w.Start == "" && w.End == ""
}

// String 返回 WireRange 的字符串表示："start-end"。
// 如果起止相同则只返回单个 IP。
// 零值返回空字符串；部分设置（仅 Start 或仅 End）返回有值的部分，避免产生
// 尾随/前导连字符（如 "10.0.0.1-" 或 "-10.0.0.1"）影响日志可读性。
func (w WireRange) String() string {
	if w.Start == w.End {
		return w.Start
	}
	if w.Start == "" {
		return w.End
	}
	if w.End == "" {
		return w.Start
	}
	return w.Start + "-" + w.End
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
		wrs[i] = WireRangeFromUnchecked(r)
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
