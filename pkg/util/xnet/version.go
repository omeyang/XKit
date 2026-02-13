package xnet

import "net/netip"

// Version 表示 IP 协议版本。
type Version uint8

const (
	// V0 表示无效或未知的 IP 版本。
	// 设计决策: 使用 V0 而非 VersionUnknown，与 V4/V6 保持简短命名一致性，
	// 且零值语义（Version(0) == V0）符合 Go 惯用法。
	V0 Version = 0
	// V4 表示 IPv4。
	V4 Version = 4
	// V6 表示 IPv6。
	V6 Version = 6
)

// String 返回版本的字符串表示。
func (v Version) String() string {
	switch v {
	case V4:
		return "IPv4"
	case V6:
		return "IPv6"
	default:
		return "unknown"
	}
}

// AddrVersion 返回 addr 的 IP 版本（V4 或 V6）。
// IPv4-mapped IPv6 地址视为 V4。
// 无效地址返回 V0。
func AddrVersion(addr netip.Addr) Version {
	if addr.Is4() || addr.Is4In6() {
		return V4
	}
	if addr.IsValid() {
		return V6
	}
	return V0
}
