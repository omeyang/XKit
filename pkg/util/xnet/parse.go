package xnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"

	"go4.org/netipx"
)

// ParseRange 从字符串解析 IP 范围。支持 4 种格式：
//   - 单 IP: "192.168.1.1"
//   - CIDR: "192.168.1.0/24"
//   - 掩码: "192.168.1.0/255.255.255.0"（仅 IPv4）
//   - 范围: "192.168.1.1-192.168.1.100"
//
// 输入会自动去除首尾空白字符。
func ParseRange(s string) (netipx.IPRange, error) {
	s = strings.TrimSpace(s)

	// 格式 4: 显式范围 "start-end"
	if idx := strings.Index(s, "-"); idx >= 0 {
		startStr := strings.TrimSpace(s[:idx])
		endStr := strings.TrimSpace(s[idx+1:])
		start, err := netip.ParseAddr(startStr)
		if err != nil {
			return netipx.IPRange{}, fmt.Errorf("%w: invalid range start: %s", ErrInvalidRange, startStr)
		}
		end, err := netip.ParseAddr(endStr)
		if err != nil {
			return netipx.IPRange{}, fmt.Errorf("%w: invalid range end: %s", ErrInvalidRange, endStr)
		}
		r := netipx.IPRangeFrom(start, end)
		if !r.IsValid() {
			return netipx.IPRange{}, fmt.Errorf("%w: %s", ErrInvalidRange, s)
		}
		return r, nil
	}

	// 格式 2/3: CIDR 或掩码 "addr/bits" 或 "addr/mask"
	if idx := strings.Index(s, "/"); idx >= 0 {
		maskStr := s[idx+1:]

		if strings.Contains(maskStr, ".") {
			return parseRangeWithMask(s[:idx], maskStr)
		}

		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return netipx.IPRange{}, fmt.Errorf("%w: invalid CIDR: %s", ErrInvalidRange, s)
		}
		return netipx.RangeOfPrefix(prefix.Masked()), nil
	}

	// 格式 1: 单 IP
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: %s", ErrInvalidRange, s)
	}
	return netipx.IPRangeFrom(addr, addr), nil
}

// parseRangeWithMask 解析掩码格式的 IP 范围（仅 IPv4），包含掩码连续性校验。
// 非连续掩码（如 "255.0.255.0"）会返回 ErrInvalidRange。
func parseRangeWithMask(addrStr, maskStr string) (netipx.IPRange, error) {
	addr, err := netip.ParseAddr(addrStr)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid address: %s", ErrInvalidRange, addrStr)
	}
	mask, err := netip.ParseAddr(maskStr)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid mask: %s", ErrInvalidRange, maskStr)
	}
	// 支持 IPv4-mapped IPv6 地址（如 ::ffff:192.168.1.0），统一转为纯 IPv4。
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if mask.Is4In6() {
		mask = mask.Unmap()
	}
	if !addr.Is4() || !mask.Is4() {
		return netipx.IPRange{}, fmt.Errorf("%w: mask notation only supports IPv4", ErrInvalidRange)
	}

	addrB := addr.As4()
	maskB := mask.As4()
	addrUint := binary.BigEndian.Uint32(addrB[:])
	maskUint := binary.BigEndian.Uint32(maskB[:])

	// 验证掩码连续性：合法掩码为前缀全 1 后缀全 0。
	inverted := ^maskUint
	if inverted&(inverted+1) != 0 {
		return netipx.IPRange{}, fmt.Errorf("%w: non-contiguous mask: %s", ErrInvalidRange, maskStr)
	}

	startUint := addrUint & maskUint
	endUint := startUint | ^maskUint

	return netipx.IPRangeFrom(AddrFromUint32(startUint), AddrFromUint32(endUint)), nil
}

// ParseRanges 从字符串切片解析并合并为 [*netipx.IPSet]。
// 每个字符串使用 [ParseRange] 解析，结果自动合并去重。
// 空切片或 nil 返回空的 IPSet。
func ParseRanges(strs []string) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for _, s := range strs {
		r, err := ParseRange(s)
		if err != nil {
			return nil, fmt.Errorf("parse range %q: %w", s, err)
		}
		b.AddRange(r)
	}
	set, err := b.IPSet()
	if err != nil {
		return nil, fmt.Errorf("build IPSet: %w", err)
	}
	return set, nil
}
