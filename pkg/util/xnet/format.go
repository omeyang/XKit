package xnet

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// FormatFullIP 将 IP 地址字符串格式化为完整长度表示。
// IPv4: "192.168.1.1" → "192.168.001.001"
// IPv6: "::1" → "00000000000000000000000000000001"
func FormatFullIP(s string) (string, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidAddress, err)
	}
	return FormatFullIPAddr(addr), nil
}

// FormatFullIPAddr 将 [netip.Addr] 格式化为完整长度表示。
// IPv4: 每段 3 位十进制，带前导零（如 "192.168.001.001"）。
// IPv6: 32 字符十六进制，无分隔符。
// 无效地址返回空字符串。
//
// 注意：IPv4-mapped IPv6 地址（如 ::ffff:192.168.1.1）会被 Unmap 为纯 IPv4 格式化，
// 因此 FormatFullIPAddr(mapped) 与 FormatFullIPAddr(unmapped) 输出相同。
// 如需保留 IPv6 表示，请先判断 [netip.Addr.Is4In6]。
func FormatFullIPAddr(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	if addr.Is4() || addr.Is4In6() {
		b := addr.Unmap().As4()
		// 手写格式化避免 fmt.Sprintf 的反射开销和额外分配。
		var buf [15]byte // "xxx.xxx.xxx.xxx"
		for i := 0; i < 4; i++ {
			off := i * 4
			if i > 0 {
				buf[off-1] = '.'
			}
			buf[off+0] = '0' + b[i]/100
			buf[off+1] = '0' + (b[i]/10)%10
			buf[off+2] = '0' + b[i]%10
		}
		return string(buf[:])
	}
	b := addr.As16()
	return hex.EncodeToString(b[:])
}

// ParseFullIP 解析完整长度的 IP 地址字符串。
// IPv4: "192.168.001.001" → netip.Addr
// IPv6: 32 字符十六进制 → netip.Addr
//
// 设计决策: 同时也接受标准格式（如 "::1"、"192.168.1.1"）作为回退，以便
// 作为通用解析入口使用（兼容旧系统中的 FullIP2IP）。如需严格匹配
// [FormatFullIPAddr] 的输出格式，请先检查字符串长度/格式再调用。
func ParseFullIP(s string) (netip.Addr, error) {
	// 尝试 IPv6 全长格式（32 个十六进制字符，无分隔符）
	if len(s) == 32 && !strings.Contains(s, ".") && !strings.Contains(s, ":") {
		b, err := hex.DecodeString(s)
		if err == nil && len(b) == 16 {
			var arr [16]byte
			copy(arr[:], b)
			return netip.AddrFrom16(arr), nil
		}
	}

	// 尝试 IPv4 带前导零格式（xxx.xxx.xxx.xxx）。
	// 设计决策: 解析失败时回退到 netip.ParseAddr，而非直接返回错误。
	// 这确保 IPv4-mapped IPv6 地址（如 "::ffff:192.168.1.1"）也能被正确解析：
	// 该格式 Split(".") 得到 4 段但首段含 ":"，strconv 解析失败后由标准库处理。
	if addr, ok := tryParsePaddedIPv4(s); ok {
		return addr, nil
	}

	// 回退到标准解析
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: %w", ErrInvalidAddress, err)
	}
	return addr, nil
}

// tryParsePaddedIPv4 尝试将 s 解析为带前导零的 IPv4 格式（如 "192.168.001.001"）。
// 仅当 s 恰好是 4 段纯十进制且每段在 [0,255] 时返回 (addr, true)。
func tryParsePaddedIPv4(s string) (netip.Addr, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return netip.Addr{}, false
	}
	var b [4]byte
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return netip.Addr{}, false
		}
		b[i] = byte(n)
	}
	return netip.AddrFrom4(b), true
}

// NormalizeIP 将 IP 地址字符串规范化为标准格式。
// 去除前导零，展开 IPv6 缩写等。
func NormalizeIP(s string) (string, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidAddress, err)
	}
	return addr.String(), nil
}

// IsValidIP 报告 s 是否为有效的 IP 地址字符串。
func IsValidIP(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}
