package xnet

import (
	"encoding/binary"
	"net/netip"
)

// 设计决策: 以下 8 个包级函数（IsPrivate ~ IsInterfaceLocalMulticast）是对
// netip.Addr 同名方法的薄包装，添加了 IsValid 前置检查。虽然 netip.Addr 零值
// 对这些方法本身也返回 false（前置检查在技术上冗余），但保留它们是为了：
//   - 与 xnet 自定义分类函数（IsRoutable, IsDocumentation 等）提供一致的包级 API
//   - 用户可从单一包导入所有分类函数，无需混用 addr.IsXxx() 和 xnet.IsYyy()

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
//	fmt.Println(c.IsGlobalUnicast) // true (私有地址也是全局单播)
func Classify(addr netip.Addr) Classification {
	if !addr.IsValid() {
		return Classification{}
	}
	return Classification{
		Version:                   AddrVersion(addr),
		IsValid:                   true,
		IsPrivate:                 addr.IsPrivate(),
		IsLoopback:                addr.IsLoopback(),
		IsLinkLocalUnicast:        addr.IsLinkLocalUnicast(),
		IsLinkLocalMulticast:      addr.IsLinkLocalMulticast(),
		IsInterfaceLocalMulticast: addr.IsInterfaceLocalMulticast(),
		IsGlobalUnicast:           addr.IsGlobalUnicast(),
		IsMulticast:               addr.IsMulticast(),
		IsUnspecified:             addr.IsUnspecified(),
		IsRoutable:                IsRoutable(addr),
		IsDocumentation:           IsDocumentation(addr),
		IsSharedAddress:           IsSharedAddress(addr),
		IsBenchmark:               IsBenchmark(addr),
		IsReserved:                IsReserved(addr),
	}
}

// Classification 包含 IP 地址的各种分类信息。
//
// 设计决策: 使用扁平的导出字段而非位标志或方法集，因为：
//   - 值类型结构体在 Go 中添加字段是向后兼容的
//   - 调用方可直接访问 c.IsPrivate，比 c.Has(FlagPrivate) 更符合 Go 惯用法
//   - 所有字段在 Classify() 一次调用中填充，避免多次方法调用开销
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

	// IsRoutable 表示是否为可路由的单播地址。
	IsRoutable bool

	// IsDocumentation 表示是否为文档专用地址（TEST-NET/2001:db8::）。
	IsDocumentation bool

	// IsSharedAddress 表示是否为共享地址空间（100.64.0.0/10, CGNAT）。
	IsSharedAddress bool

	// IsBenchmark 表示是否为基准测试地址（IPv4: 198.18.0.0/15, IPv6: 2001:2::/48）。
	IsBenchmark bool

	// IsReserved 表示是否为保留地址（240.0.0.0/4, Class E）。
	IsReserved bool
}

// String 返回分类信息的字符串表示。
// 优先级：越特殊的分类越靠前（如 loopback > private > global-unicast）。
func (c Classification) String() string {
	if !c.IsValid {
		return "invalid"
	}

	// 按优先级排列，第一个匹配的即为结果
	labels := [...]struct {
		flag  bool
		label string
	}{
		{c.IsLoopback, "loopback"},
		{c.IsUnspecified, "unspecified"},
		{c.IsPrivate, "private"},
		{c.IsLinkLocalUnicast, "link-local-unicast"},
		{c.IsLinkLocalMulticast, "link-local-multicast"},
		{c.IsInterfaceLocalMulticast, "interface-local-multicast"},
		{c.IsDocumentation, "documentation"},
		{c.IsSharedAddress, "shared-address"},
		{c.IsBenchmark, "benchmark"},
		{c.IsReserved, "reserved"},
		{c.IsMulticast, "multicast"},
		{c.IsGlobalUnicast, "global-unicast"},
	}

	for _, e := range labels {
		if e.flag {
			return e.label
		}
	}
	// 防御性分支：Classify() 对有效地址总会设置至少一个标志（IsGlobalUnicast 兜底），
	// 此行仅在手工构造 Classification{IsValid: true} 时触达。
	return "unknown"
}

// IsRoutable 报告 addr 是否为可路由的单播地址。
// 可路由地址是指：
//   - 有效
//   - 非环回
//   - 非链路本地单播
//   - 非未指定
//   - 非多播（多播使用独立的组播路由机制，不属于常规单播路由）
//   - 非有限广播（255.255.255.255，仅本地链路广播，不可被路由器转发）
//
// 注意：私有地址虽然不可在公网路由，但在局域网内可路由，
// 因此 IsRoutable 返回 true。如需判断公网可路由，请使用 IsGlobalUnicast。
//
// 设计决策: IsRoutable 仅检查协议层面的路由能力（排除环回、链路本地等），
// 不排除 RFC 策略层面禁止路由的地址（文档地址 192.0.2.0/24、基准测试地址
// 198.18.0.0/15、保留地址 240.0.0.0/4）。这些地址在技术上可被路由器转发，
// 但 RFC 规定不应出现在实际路由表中。如需更严格过滤，请组合使用
// [IsDocumentation]、[IsBenchmark]、[IsReserved] 进一步排除。
func IsRoutable(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	// 排除 IPv4 有限广播地址 (255.255.255.255)
	if (addr.Is4() || addr.Is4In6()) && ipv4ToUint32(addr) == 0xFFFFFFFF {
		return false
	}
	return !addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsUnspecified() &&
		!addr.IsMulticast()
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

// inRange 检查 v 是否在 [lo, hi] 范围内。
func inRange(v, lo, hi uint32) bool {
	return v >= lo && v <= hi
}

// ipv4ToUint32 将 IPv4 地址转换为 uint32。
// 调用前必须确保 addr.Is4() || addr.Is4In6() 为 true。
//
// 与导出的 [AddrToUint32] 功能相似，区别在于：
//   - ipv4ToUint32 是内部函数，不做类型检查（调用方已保证类型）
//   - AddrToUint32 是导出函数，返回 (uint32, bool) 以指示类型是否兼容
//
// 两者都使用 binary.BigEndian.Uint32 实现以保持一致性。
func ipv4ToUint32(addr netip.Addr) uint32 {
	b := addr.Unmap().As4()
	return binary.BigEndian.Uint32(b[:])
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
// 基准测试地址包括：
//   - IPv4: 198.18.0.0/15 (RFC 2544)
//   - IPv6: 2001:2::/48 (RFC 5180)
//
// 无效地址返回 false。
func IsBenchmark(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.Is4() || addr.Is4In6() {
		v := ipv4ToUint32(addr)
		// 198.18.0.0/15 = 0xC6120000 - 0xC613FFFF
		return v >= 0xC6120000 && v <= 0xC613FFFF
	}
	return isIPv6Benchmark(addr)
}

// isIPv6Benchmark 检查 IPv6 地址是否为基准测试地址 (2001:0002::/48, RFC 5180)。
func isIPv6Benchmark(addr netip.Addr) bool {
	b := addr.As16()
	// 检查前 6 字节是否为 2001:0002:0000（即 /48 前缀）
	// 使用数组切片语法确保类型安全（与 isIPv6Documentation 一致）
	prefix := [6]byte{b[0], b[1], b[2], b[3], b[4], b[5]}
	return prefix == [6]byte{0x20, 0x01, 0x00, 0x02, 0x00, 0x00}
}

// IsReserved 报告 addr 是否为保留地址（Class E）。
// 保留地址：240.0.0.0/4
// 保留用于未来使用，RFC 1112 定义。
//
// 仅适用于 IPv4，无效地址或 IPv6 地址返回 false。
func IsReserved(addr netip.Addr) bool {
	if !addr.Is4() && !addr.Is4In6() {
		return false
	}
	v := ipv4ToUint32(addr)
	// 240.0.0.0/4 = 0xF0000000 - 0xFFFFFFFF
	return v >= 0xF0000000
}
