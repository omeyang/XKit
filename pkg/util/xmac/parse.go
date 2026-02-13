package xmac

import (
	"fmt"
	"net"
	"strings"
)

// Parse 解析 MAC 地址字符串。
//
// 支持的格式：
//   - 冒号分隔：aa:bb:cc:dd:ee:ff, AA:BB:CC:DD:EE:FF
//   - 短线分隔：aa-bb-cc-dd-ee-ff, AA-BB-CC-DD-EE-FF
//   - 点分隔：aabb.ccdd.eeff, AABB.CCDD.EEFF
//   - 无分隔：aabbccddeeff, AABBCCDDEEFF
//
// 输入会自动去除首尾空白。大小写不敏感，结果统一小写存储。
//
// 注意：全零 MAC "00:00:00:00:00:00" 返回零值 Addr{} 且无错误，
// 零值 [Addr.IsValid] 返回 false。如需验证业务可用性，请使用 [Addr.IsUsable]。
func Parse(s string) (Addr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Addr{}, ErrEmpty
	}

	// 尝试无分隔符格式（12 个十六进制字符）
	if len(s) == 12 && !containsSeparator(s) {
		return parseNoSeparator(s)
	}

	// 尝试冒号/短线分隔格式（17 字符：xx:xx:xx:xx:xx:xx 或 xx-xx-xx-xx-xx-xx）。
	// 自行解析避免 net.ParseMAC 的 []byte 堆分配。
	if len(s) == 17 {
		sep := s[2]
		if sep == ':' || sep == '-' {
			return parseWithSeparator(s, sep)
		}
	}

	// 尝试点分隔格式（14 字符：xxxx.xxxx.xxxx，Cisco 风格）。
	// 自行解析避免 net.ParseMAC 的 []byte 堆分配。
	if len(s) == 14 && s[4] == '.' && s[9] == '.' {
		return parseDot(s)
	}

	// 其他格式回退到标准库
	return parseStdlib(s)
}

// MustParse 类似 [Parse]，但解析失败时 panic。
// 仅用于包级常量初始化或测试。
func MustParse(s string) Addr {
	addr, err := Parse(s)
	if err != nil {
		panic(fmt.Sprintf("xmac.MustParse(%q): %v", s, err))
	}
	return addr
}

// ParseBytes 从字节切片创建 MAC 地址。
// 切片长度必须为 6。
func ParseBytes(b []byte) (Addr, error) {
	if len(b) != 6 {
		return Addr{}, fmt.Errorf("%w: expected 6 bytes, got %d", ErrInvalidLength, len(b))
	}
	var addr Addr
	copy(addr.bytes[:], b)
	return addr, nil
}

// FromHardwareAddr 从 [net.HardwareAddr] 创建 MAC 地址。
// 长度必须为 6 字节。
func FromHardwareAddr(hw net.HardwareAddr) (Addr, error) {
	return ParseBytes([]byte(hw))
}

// parseWithSeparator 解析 17 字符的冒号/短线分隔格式（xx:xx:xx:xx:xx:xx）。
// 零堆分配，比 net.ParseMAC 更高效。
func parseWithSeparator(s string, sep byte) (Addr, error) {
	// 验证分隔符位置：索引 2, 5, 8, 11, 14
	if s[5] != sep || s[8] != sep || s[11] != sep || s[14] != sep {
		return Addr{}, fmt.Errorf("%w: inconsistent separators", ErrInvalidFormat)
	}

	var addr Addr
	for i := range 6 {
		offset := i * 3 // 每组 2 个十六进制字符 + 1 个分隔符
		b, err := parseHexByte(s[offset], s[offset+1])
		if err != nil {
			return Addr{}, fmt.Errorf("%w: invalid hex at position %d", ErrInvalidFormat, offset)
		}
		addr.bytes[i] = b
	}
	return addr, nil
}

// parseDot 解析 14 字符的点分隔格式（xxxx.xxxx.xxxx，Cisco 风格）。
// 零堆分配，比 net.ParseMAC 更高效。
func parseDot(s string) (Addr, error) {
	// 位置映射：0123.5678.abcd（索引 4 和 9 是点）
	// 每个字节对在字符串中的起始偏移量
	offsets := [6]int{0, 2, 5, 7, 10, 12}
	var addr Addr
	for i, off := range offsets {
		b, err := parseHexByte(s[off], s[off+1])
		if err != nil {
			return Addr{}, fmt.Errorf("%w: invalid hex at position %d", ErrInvalidFormat, off)
		}
		addr.bytes[i] = b
	}
	return addr, nil
}

// parseStdlib 使用标准库 [net.ParseMAC] 解析不常见格式。
// 仅在自定义解析器都不匹配时作为回退。
func parseStdlib(s string) (Addr, error) {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return Addr{}, fmt.Errorf("%w: %w", ErrInvalidFormat, err)
	}
	// 验证长度（net.ParseMAC 支持 EUI-64，我们只支持 EUI-48）
	if len(hw) != 6 {
		return Addr{}, fmt.Errorf("%w: expected 6 bytes, got %d", ErrInvalidLength, len(hw))
	}
	var addr Addr
	copy(addr.bytes[:], hw)
	return addr, nil
}

// parseNoSeparator 解析无分隔符的 12 字符十六进制字符串。
func parseNoSeparator(s string) (Addr, error) {
	var addr Addr
	for i := range 6 {
		b, err := parseHexByte(s[i*2], s[i*2+1])
		if err != nil {
			return Addr{}, fmt.Errorf("%w: invalid hex at position %d", ErrInvalidFormat, i*2)
		}
		addr.bytes[i] = b
	}
	return addr, nil
}

// containsSeparator 检查字符串是否包含 MAC 地址分隔符。
func containsSeparator(s string) bool {
	return strings.ContainsAny(s, ":-.")
}

// parseHexByte 解析两个十六进制字符为一个字节。
func parseHexByte(high, low byte) (byte, error) {
	h := hexValue(high)
	l := hexValue(low)
	if h < 0 || l < 0 {
		return 0, ErrInvalidFormat
	}
	return byte(h<<4 | l), nil
}

// hexValue 返回十六进制字符的数值，无效字符返回 -1。
func hexValue(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	default:
		return -1
	}
}
