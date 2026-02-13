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

	// 设计决策: 拒绝包含 IPv6 zone ID 的输入（如 fe80::1%eth0）。
	// netipx.IPRange/IPSet 会静默丢弃 zone 信息，导致规则匹配失败
	// （ACL/白名单/黑名单误判），属于高风险正确性问题。
	// 在 IP 地址字符串中 '%' 仅用作 zone 分隔符，因此检查 '%' 即可。
	if strings.Contains(s, "%") {
		return netipx.IPRange{}, fmt.Errorf("%w: IPv6 zone ID is not supported in range operations: %s", ErrInvalidRange, s)
	}

	// 格式 4: 显式范围 "start-end"
	if idx := strings.Index(s, "-"); idx >= 0 {
		r, err, handled := parseExplicitRange(s, idx)
		if handled {
			return r, err
		}
		// 两侧都无效 → 回退到 CIDR / 单 IP 分支
	}

	// 格式 2/3: CIDR 或掩码 "addr/bits" 或 "addr/mask"
	if idx := strings.Index(s, "/"); idx >= 0 {
		addrPart := strings.TrimSpace(s[:idx])
		maskStr := strings.TrimSpace(s[idx+1:])

		if strings.Contains(maskStr, ".") {
			return parseRangeWithMask(addrPart, maskStr)
		}

		// 重新组装去除空白后的 CIDR 字符串
		cidr := addrPart + "/" + maskStr
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return netipx.IPRange{}, fmt.Errorf("%w: invalid CIDR: %w", ErrInvalidRange, err)
		}
		return netipx.RangeOfPrefix(prefix.Masked()), nil
	}

	// 格式 1: 单 IP
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: %w", ErrInvalidRange, err)
	}
	return netipx.IPRangeFrom(addr, addr), nil
}

// parseExplicitRange 尝试将 s 按位置 idx 处的 '-' 拆分为起止地址。
// 返回 (range, err, handled)：handled=true 时结果已确定，handled=false 时调用方应回退。
func parseExplicitRange(s string, idx int) (netipx.IPRange, error, bool) {
	startStr := strings.TrimSpace(s[:idx])
	endStr := strings.TrimSpace(s[idx+1:])
	start, startErr := netip.ParseAddr(startStr)
	end, endErr := netip.ParseAddr(endStr)
	if startErr == nil && endErr == nil {
		r := netipx.IPRangeFrom(start, end)
		if !r.IsValid() {
			return netipx.IPRange{}, fmt.Errorf("%w: %s", ErrInvalidRange, s), true
		}
		return r, nil, true
	}
	// 仅一侧解析成功 → 明确是范围格式但另一端无效，返回具体错误
	if startErr == nil {
		// start 有效但 end 无效 — 可能是 zone ID 含 '-' 的单地址，先尝试整体解析。
		//
		// 设计决策: IPv6 zone ID 可包含任意字符（包括 '-'），例如 "fe80::1%eth-0" 或
		// "fe80::1%br-lan-0" 都是合法地址。当输入形如 "fe80::1%eth-0-garbage" 时，
		// 拆分后 start="fe80::1%eth" 可解析，end="0-garbage" 不可解析，
		// 但整体字符串会被 netip.ParseAddr 解析为 zone="eth-0-garbage" 的合法地址。
		// 此行为是正确的：zone ID 由操作系统/接口定义，无法在解析器层面限制其内容。
		if addr, err := netip.ParseAddr(s); err == nil {
			return netipx.IPRangeFrom(addr, addr), nil, true
		}
		return netipx.IPRange{}, fmt.Errorf("%w: invalid range end: %s", ErrInvalidRange, endStr), true
	}
	if endErr == nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid range start: %s", ErrInvalidRange, startStr), true
	}
	// 两侧都无效 → 不处理，让调用方回退
	return netipx.IPRange{}, nil, false
}

// parseRangeWithMask 解析掩码格式的 IP 范围（仅 IPv4），包含掩码连续性校验。
// 非连续掩码（如 "255.0.255.0"）会返回 ErrInvalidRange。
func parseRangeWithMask(addrStr, maskStr string) (netipx.IPRange, error) {
	addr, err := netip.ParseAddr(addrStr)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid address: %w", ErrInvalidRange, err)
	}
	mask, err := netip.ParseAddr(maskStr)
	if err != nil {
		return netipx.IPRange{}, fmt.Errorf("%w: invalid mask: %w", ErrInvalidRange, err)
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
