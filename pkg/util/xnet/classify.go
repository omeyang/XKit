package xnet

import "net/netip"

// IsPrivate 报告 addr 是否为私有地址。
// 私有地址包括：
//   - IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
//   - IPv6: fc00::/7 (Unique Local Addresses)
//
// 这是对 [netip.Addr.IsPrivate] 的包装。
// 无效地址返回 false。
func IsPrivate(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsPrivate()
}

// IsLoopback 报告 addr 是否为环回地址。
// 环回地址包括：
//   - IPv4: 127.0.0.0/8
//   - IPv6: ::1
//
// 这是对 [netip.Addr.IsLoopback] 的包装。
// 无效地址返回 false。
func IsLoopback(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsLoopback()
}

// IsLinkLocalUnicast 报告 addr 是否为链路本地单播地址。
// 链路本地单播地址包括：
//   - IPv4: 169.254.0.0/16 (APIPA)
//   - IPv6: fe80::/10
//
// 这是对 [netip.Addr.IsLinkLocalUnicast] 的包装。
// 无效地址返回 false。
func IsLinkLocalUnicast(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsLinkLocalUnicast()
}

// IsLinkLocalMulticast 报告 addr 是否为链路本地多播地址。
// 链路本地多播地址包括：
//   - IPv4: 224.0.0.0/24
//   - IPv6: ff02::/16
//
// 这是对 [netip.Addr.IsLinkLocalMulticast] 的包装。
// 无效地址返回 false。
func IsLinkLocalMulticast(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsLinkLocalMulticast()
}

// IsGlobalUnicast 报告 addr 是否为全局单播地址。
// 全局单播地址是指不属于以下任何类别的地址：
//   - 无效地址
//   - 未指定地址 (0.0.0.0 或 ::)
//   - 环回地址
//   - 多播地址
//   - 链路本地单播地址
//
// 这是对 [netip.Addr.IsGlobalUnicast] 的包装。
// 无效地址返回 false。
func IsGlobalUnicast(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsGlobalUnicast()
}

// IsMulticast 报告 addr 是否为多播地址。
// 多播地址包括：
//   - IPv4: 224.0.0.0/4
//   - IPv6: ff00::/8
//
// 这是对 [netip.Addr.IsMulticast] 的包装。
// 无效地址返回 false。
func IsMulticast(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsMulticast()
}

// IsUnspecified 报告 addr 是否为未指定地址。
// 未指定地址是：
//   - IPv4: 0.0.0.0
//   - IPv6: ::
//
// 这是对 [netip.Addr.IsUnspecified] 的包装。
// 无效地址返回 false。
func IsUnspecified(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsUnspecified()
}

// IsInterfaceLocalMulticast 报告 addr 是否为接口本地多播地址。
// 仅适用于 IPv6，返回 scope 为 1 (interface-local) 的多播地址。
//
// 这是对 [netip.Addr.IsInterfaceLocalMulticast] 的包装。
// 无效地址返回 false。
func IsInterfaceLocalMulticast(addr netip.Addr) bool {
	return addr.IsValid() && addr.IsInterfaceLocalMulticast()
}

// Classify 返回 IP 地址的分类信息。
// 返回一个包含各种分类结果的结构体。
//
// 示例：
//
//	addr := netip.MustParseAddr("192.168.1.1")
//	c := xnet.Classify(addr)
//	fmt.Println(c.IsPrivate)      // true
//	fmt.Println(c.IsGlobalUnicast) // false
func Classify(addr netip.Addr) Classification {
	if !addr.IsValid() {
		return Classification{}
	}
	return Classification{
		Version:                 AddrVersion(addr),
		IsValid:                 true,
		IsPrivate:               addr.IsPrivate(),
		IsLoopback:              addr.IsLoopback(),
		IsLinkLocalUnicast:      addr.IsLinkLocalUnicast(),
		IsLinkLocalMulticast:    addr.IsLinkLocalMulticast(),
		IsInterfaceLocalMulticast: addr.IsInterfaceLocalMulticast(),
		IsGlobalUnicast:         addr.IsGlobalUnicast(),
		IsMulticast:             addr.IsMulticast(),
		IsUnspecified:           addr.IsUnspecified(),
	}
}

// Classification 包含 IP 地址的各种分类信息。
type Classification struct {
	// Version 是 IP 版本（V4 或 V6）。
	Version Version

	// IsValid 表示地址是否有效。
	IsValid bool

	// IsPrivate 表示是否为私有地址。
	IsPrivate bool

	// IsLoopback 表示是否为环回地址。
	IsLoopback bool

	// IsLinkLocalUnicast 表示是否为链路本地单播地址。
	IsLinkLocalUnicast bool

	// IsLinkLocalMulticast 表示是否为链路本地多播地址。
	IsLinkLocalMulticast bool

	// IsInterfaceLocalMulticast 表示是否为接口本地多播地址（仅 IPv6）。
	IsInterfaceLocalMulticast bool

	// IsGlobalUnicast 表示是否为全局单播地址。
	IsGlobalUnicast bool

	// IsMulticast 表示是否为多播地址。
	IsMulticast bool

	// IsUnspecified 表示是否为未指定地址。
	IsUnspecified bool
}

// String 返回分类信息的字符串表示。
func (c Classification) String() string {
	if !c.IsValid {
		return "invalid"
	}

	switch {
	case c.IsLoopback:
		return "loopback"
	case c.IsUnspecified:
		return "unspecified"
	case c.IsPrivate:
		return "private"
	case c.IsLinkLocalUnicast:
		return "link-local-unicast"
	case c.IsLinkLocalMulticast:
		return "link-local-multicast"
	case c.IsInterfaceLocalMulticast:
		return "interface-local-multicast"
	case c.IsMulticast:
		return "multicast"
	case c.IsGlobalUnicast:
		return "global-unicast"
	default:
		return "unknown"
	}
}

// IsRoutable 报告 addr 是否为可路由地址。
// 可路由地址是指：
//   - 有效
//   - 非环回
//   - 非链路本地
//   - 非未指定
//
// 注意：私有地址虽然不可在公网路由，但在局域网内可路由，
// 因此 IsRoutable 返回 true。如需判断公网可路由，请使用 IsGlobalUnicast。
func IsRoutable(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	return !addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsUnspecified()
}

// IsDocumentation 报告 addr 是否为文档专用地址。
// 文档专用地址用于文档和示例，不应在实际网络中使用。
//   - IPv4: 192.0.2.0/24 (TEST-NET-1), 198.51.100.0/24 (TEST-NET-2), 203.0.113.0/24 (TEST-NET-3)
//   - IPv6: 2001:db8::/32
//
// 无效地址返回 false。
func IsDocumentation(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}

	if addr.Is4() || addr.Is4In6() {
		return isIPv4Documentation(addr)
	}

	// IPv6: 2001:db8::/32
	return isIPv6Documentation(addr)
}

// isIPv4Documentation 检查 IPv4 地址是否为文档专用地址。
// 调用前必须确保 addr.Is4() || addr.Is4In6() 为 true。
func isIPv4Documentation(addr netip.Addr) bool {
	v := ipv4ToUint32(addr)
	// 192.0.2.0/24 (TEST-NET-1): 0xC0000200 - 0xC00002FF
	// 198.51.100.0/24 (TEST-NET-2): 0xC6336400 - 0xC63364FF
	// 203.0.113.0/24 (TEST-NET-3): 0xCB007100 - 0xCB0071FF
	return inRange(v, 0xC0000200, 0xC00002FF) ||
		inRange(v, 0xC6336400, 0xC63364FF) ||
		inRange(v, 0xCB007100, 0xCB0071FF)
}

// isIPv6Documentation 检查 IPv6 地址是否为文档专用地址 (2001:db8::/32)。
func isIPv6Documentation(addr netip.Addr) bool {
	b := addr.As16()
	// 检查前 4 字节是否为 2001:0db8
	// 使用数组切片语法确保类型安全
	prefix := [4]byte{b[0], b[1], b[2], b[3]}
	return prefix == [4]byte{0x20, 0x01, 0x0d, 0xb8}
}

// inRange 检查 v 是否在 [min, max] 范围内。
func inRange(v, min, max uint32) bool {
	return v >= min && v <= max
}

// ipv4ToUint32 将 IPv4 地址转换为 uint32。
// 调用前必须确保 addr.Is4() || addr.Is4In6() 为 true。
//
// 注意：此函数与 [AddrToUint32] 功能相似，但有以下区别：
//   - ipv4ToUint32 是内部函数，不做类型检查（调用方已保证类型）
//   - AddrToUint32 是导出函数，返回 (uint32, bool) 以指示类型是否兼容
//
// 这种设计避免了在已知类型的热路径上重复检查，提升性能。
func ipv4ToUint32(addr netip.Addr) uint32 {
	b := addr.Unmap().As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// IsSharedAddress 报告 addr 是否为共享地址空间（Carrier-Grade NAT）。
// 共享地址空间：100.64.0.0/10
// 用于运营商级 NAT (CGNAT/LSN)，RFC 6598 定义。
//
// 仅适用于 IPv4，无效地址或 IPv6 地址返回 false。
func IsSharedAddress(addr netip.Addr) bool {
	if !addr.Is4() && !addr.Is4In6() {
		return false
	}
	v := ipv4ToUint32(addr)
	// 100.64.0.0/10 = 0x64400000 - 0x647FFFFF
	return v >= 0x64400000 && v <= 0x647FFFFF
}

// IsBenchmark 报告 addr 是否为基准测试地址。
// 基准测试地址：198.18.0.0/15
// 用于网络设备基准测试，RFC 2544 定义。
//
// 仅适用于 IPv4，无效地址或 IPv6 地址返回 false。
func IsBenchmark(addr netip.Addr) bool {
	if !addr.Is4() && !addr.Is4In6() {
		return false
	}
	v := ipv4ToUint32(addr)
	// 198.18.0.0/15 = 0xC6120000 - 0xC613FFFF
	return v >= 0xC6120000 && v <= 0xC613FFFF
}
