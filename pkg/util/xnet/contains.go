package xnet

import (
	"fmt"
	"math/big"
	"net/netip"

	"go4.org/netipx"
)

// RangeContainsV4 使用 uint32 比较优化 IPv4 范围查询。
// 对于 IPv4 地址，直接比较 uint32 值比通过 netip.Addr.Compare 更高效。
// 非 IPv4 地址返回 false。
func RangeContainsV4(from, to, addr netip.Addr) bool {
	fromU, ok1 := AddrToUint32(from)
	toU, ok2 := AddrToUint32(to)
	addrU, ok3 := AddrToUint32(addr)
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	return addrU >= fromU && addrU <= toU
}

// IPSetFromRanges 从 IPRange 切片构建 IPSet。
// 自动合并重叠和相邻的范围。
// 无效范围（From > To 或混合地址族）会导致 IPSet() 返回错误。
// 如需显式校验每个范围，请使用 [IPSetFromRangesStrict]。
// 空切片返回空的 IPSet（非 nil）。
func IPSetFromRanges(ranges []netipx.IPRange) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for _, r := range ranges {
		b.AddRange(r)
	}
	return b.IPSet()
}

// IPSetFromRangesStrict 从 IPRange 切片构建 IPSet，严格校验每个范围。
// 自动合并重叠和相邻的范围。
// 如果任何范围无效（From > To 或混合地址族），返回错误。
// 空切片返回空的 IPSet（非 nil）。
func IPSetFromRangesStrict(ranges []netipx.IPRange) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for i, r := range ranges {
		if !r.IsValid() {
			return nil, fmt.Errorf("%w: range [%d] %s-%s is invalid", ErrInvalidRange, i, r.From(), r.To())
		}
		b.AddRange(r)
	}
	return b.IPSet()
}

// MergeRanges 合并重叠和相邻的 IP 范围。
// 返回的范围已排序且不重叠。
// 内部使用 IPSet 实现，保证正确性。
//
// 当输入包含无效范围（如 From > To 或混合地址族）时返回错误。
// 空切片或 nil 返回 (nil, nil)。
func MergeRanges(ranges []netipx.IPRange) ([]netipx.IPRange, error) {
	if len(ranges) == 0 {
		return nil, nil
	}
	var b netipx.IPSetBuilder
	for i, r := range ranges {
		if !r.IsValid() {
			return nil, fmt.Errorf("%w: range [%d] %s-%s is invalid", ErrInvalidRange, i, r.From(), r.To())
		}
		b.AddRange(r)
	}
	set, err := b.IPSet()
	if err != nil {
		return nil, fmt.Errorf("merge ranges: %w", err)
	}
	return set.Ranges(), nil
}

// RangeSize 计算 IP 范围包含的地址数量。
// 返回的 big.Int 值为 To - From + 1。
// 无效范围返回 nil。
func RangeSize(r netipx.IPRange) *big.Int {
	if !r.IsValid() {
		return nil
	}
	from := AddrToBigInt(r.From())
	to := AddrToBigInt(r.To())
	// size = to - from + 1
	size := new(big.Int).Sub(to, from)
	return size.Add(size, big.NewInt(1))
}

// RangeSizeUint64 计算 IPv4 范围包含的地址数量。
// 仅适用于 IPv4 范围，且结果不超过 uint64 最大值。
// 非 IPv4 范围或无效范围返回 (0, false)。
func RangeSizeUint64(r netipx.IPRange) (uint64, bool) {
	if !r.IsValid() {
		return 0, false
	}
	fromU, ok1 := AddrToUint32(r.From())
	toU, ok2 := AddrToUint32(r.To())
	if !ok1 || !ok2 {
		return 0, false
	}
	return uint64(toU-fromU) + 1, true
}

// RangeToPrefix 尝试将 IP 范围转换为单个 CIDR 前缀。
// 如果范围恰好对应一个 CIDR 块（如 192.168.1.0-192.168.1.255 对应 /24），
// 返回该 Prefix 和 true。
// 如果范围无法用单个 CIDR 表示，返回零值和 false。
//
// 示例：
//
//	r, _ := xnet.ParseRange("192.168.1.0-192.168.1.255")
//	prefix, ok := xnet.RangeToPrefix(r)  // 192.168.1.0/24, true
//
//	r, _ = xnet.ParseRange("192.168.1.1-192.168.1.100")
//	prefix, ok = xnet.RangeToPrefix(r)   // netip.Prefix{}, false
func RangeToPrefix(r netipx.IPRange) (netip.Prefix, bool) {
	if !r.IsValid() {
		return netip.Prefix{}, false
	}
	// netipx.IPRange.Prefix() 返回恰好覆盖该范围的 Prefix（如果存在）
	prefix, ok := r.Prefix()
	if !ok {
		return netip.Prefix{}, false
	}
	// 设计决策: 以下验证在理论上是冗余的（netipx.IPRange.Prefix() 在 ok=true 时
	// 保证前缀与范围完全一致），但作为防御性检查保留，以应对未来 netipx 库行为变更。
	prefixRange := netipx.RangeOfPrefix(prefix)
	if prefixRange.From().Compare(r.From()) != 0 || prefixRange.To().Compare(r.To()) != 0 {
		return netip.Prefix{}, false
	}
	return prefix, true
}

// RangeToPrefixes 将 IP 范围分解为最少数量的 CIDR 前缀。
// 任何 IP 范围都可以表示为若干 CIDR 块的并集。
//
// 示例：
//
//	r, _ := xnet.ParseRange("192.168.1.0-192.168.1.255")
//	prefixes := xnet.RangeToPrefixes(r)  // [192.168.1.0/24]
//
//	r, _ = xnet.ParseRange("192.168.1.1-192.168.1.100")
//	prefixes = xnet.RangeToPrefixes(r)   // [192.168.1.1/32, 192.168.1.2/31, ...]
func RangeToPrefixes(r netipx.IPRange) []netip.Prefix {
	if !r.IsValid() {
		return nil
	}
	return r.Prefixes()
}
