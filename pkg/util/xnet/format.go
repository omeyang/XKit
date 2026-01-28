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
		return "", fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	return FormatFullIPAddr(addr), nil
}

// FormatFullIPAddr 将 [netip.Addr] 格式化为完整长度表示。
// IPv4: 每段 3 位十进制，带前导零（如 "192.168.001.001"）。
// IPv6: 32 字符十六进制，无分隔符。
// 无效地址返回空字符串。
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
// 同时也接受标准格式作为回退。
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

	// 尝试 IPv4 带前导零格式（xxx.xxx.xxx.xxx）
	if parts := strings.Split(s, "."); len(parts) == 4 {
		var b [4]byte
		for i, p := range parts {
			n, err := strconv.ParseUint(p, 10, 8)
			if err != nil {
				return netip.Addr{}, fmt.Errorf("%w: invalid octet %q", ErrInvalidAddress, p)
			}
			b[i] = byte(n)
		}
		return netip.AddrFrom4(b), nil
	}

	// 回退到标准解析
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	return addr, nil
}

// NormalizeIP 将 IP 地址字符串规范化为标准格式。
// 去除前导零，展开 IPv6 缩写等。
func NormalizeIP(s string) (string, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	return addr.String(), nil
}

// IsValidIP 报告 s 是否为有效的 IP 地址字符串。
func IsValidIP(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}
