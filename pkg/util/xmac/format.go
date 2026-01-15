package xmac

// Format 定义 MAC 地址的格式化风格。
type Format uint8

const (
	// FormatColon 使用冒号分隔，小写：aa:bb:cc:dd:ee:ff
	FormatColon Format = iota
	// FormatDash 使用短线分隔，小写：aa-bb-cc-dd-ee-ff
	FormatDash
	// FormatDot 使用点分隔（Cisco 风格），小写：aabb.ccdd.eeff
	FormatDot
	// FormatBare 无分隔符，小写：aabbccddeeff
	FormatBare
	// FormatColonUpper 使用冒号分隔，大写：AA:BB:CC:DD:EE:FF
	FormatColonUpper
	// FormatDashUpper 使用短线分隔，大写：AA-BB-CC-DD-EE-FF
	FormatDashUpper
)

// 十六进制字符表。
const (
	hexLower = "0123456789abcdef"
	hexUpper = "0123456789ABCDEF"
)

// String 返回默认格式（小写冒号）的字符串表示。
// 无效地址返回空字符串。
func (a Addr) String() string {
	if !a.IsValid() {
		return ""
	}
	return a.FormatString(FormatColon)
}

// FormatString 按指定格式返回 MAC 地址字符串。
// 无效地址返回空字符串。
func (a Addr) FormatString(f Format) string {
	if !a.IsValid() {
		return ""
	}

	switch f {
	case FormatColon:
		return formatWithSep(a.bytes, ':', hexLower)
	case FormatDash:
		return formatWithSep(a.bytes, '-', hexLower)
	case FormatDot:
		return formatDot(a.bytes, hexLower)
	case FormatBare:
		return formatBare(a.bytes, hexLower)
	case FormatColonUpper:
		return formatWithSep(a.bytes, ':', hexUpper)
	case FormatDashUpper:
		return formatWithSep(a.bytes, '-', hexUpper)
	default:
		return formatWithSep(a.bytes, ':', hexLower)
	}
}

// formatWithSep 使用指定分隔符格式化（xx:xx:xx:xx:xx:xx 或 xx-xx-xx-xx-xx-xx）。
// 预分配精确大小，零额外分配。
func formatWithSep(b [6]byte, sep byte, hex string) string {
	// 6*2 + 5 = 17 字节
	var buf [17]byte
	buf[0] = hex[b[0]>>4]
	buf[1] = hex[b[0]&0x0f]
	buf[2] = sep
	buf[3] = hex[b[1]>>4]
	buf[4] = hex[b[1]&0x0f]
	buf[5] = sep
	buf[6] = hex[b[2]>>4]
	buf[7] = hex[b[2]&0x0f]
	buf[8] = sep
	buf[9] = hex[b[3]>>4]
	buf[10] = hex[b[3]&0x0f]
	buf[11] = sep
	buf[12] = hex[b[4]>>4]
	buf[13] = hex[b[4]&0x0f]
	buf[14] = sep
	buf[15] = hex[b[5]>>4]
	buf[16] = hex[b[5]&0x0f]
	return string(buf[:])
}

// formatDot 格式化为点分隔格式（xxxx.xxxx.xxxx）。
func formatDot(b [6]byte, hex string) string {
	// 4+1+4+1+4 = 14 字节
	var buf [14]byte
	buf[0] = hex[b[0]>>4]
	buf[1] = hex[b[0]&0x0f]
	buf[2] = hex[b[1]>>4]
	buf[3] = hex[b[1]&0x0f]
	buf[4] = '.'
	buf[5] = hex[b[2]>>4]
	buf[6] = hex[b[2]&0x0f]
	buf[7] = hex[b[3]>>4]
	buf[8] = hex[b[3]&0x0f]
	buf[9] = '.'
	buf[10] = hex[b[4]>>4]
	buf[11] = hex[b[4]&0x0f]
	buf[12] = hex[b[5]>>4]
	buf[13] = hex[b[5]&0x0f]
	return string(buf[:])
}

// formatBare 格式化为无分隔符格式（xxxxxxxxxxxx）。
func formatBare(b [6]byte, hex string) string {
	var buf [12]byte
	buf[0] = hex[b[0]>>4]
	buf[1] = hex[b[0]&0x0f]
	buf[2] = hex[b[1]>>4]
	buf[3] = hex[b[1]&0x0f]
	buf[4] = hex[b[2]>>4]
	buf[5] = hex[b[2]&0x0f]
	buf[6] = hex[b[3]>>4]
	buf[7] = hex[b[3]&0x0f]
	buf[8] = hex[b[4]>>4]
	buf[9] = hex[b[4]&0x0f]
	buf[10] = hex[b[5]>>4]
	buf[11] = hex[b[5]&0x0f]
	return string(buf[:])
}
