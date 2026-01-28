// Package xnet 提供 IP 地址工具库。
//
// xnet 基于 Go 标准库 [net/netip] 和社区库 [go4.org/netipx] 构建，
// 直接使用 [netip.Addr] 和 [netipx.IPRange] / [*netipx.IPSet]，
// 提供 IP 版本判断、转换、格式化、解析和序列化等增量工具函数。
//
// # 核心功能
//
//   - version.go: IP 版本类型 [Version] 及 [AddrVersion] 判断函数
//   - convert.go: uint32/BigInt 与 [netip.Addr] 互转、IPv4/IPv6 映射转换、地址加减运算
//   - format.go: FullIP 全长格式化（"192.168.001.001"）、标准化、校验
//   - parse.go: 解析单 IP/CIDR/掩码/范围格式为 [netipx.IPRange]，批量解析为 [*netipx.IPSet]
//   - wire.go: [WireRange] JSON/BSON/YAML 序列化的 IP 范围结构
//   - contains.go: IP 范围判断、合并、大小计算等辅助函数
//
// # 快速示例
//
// 解析和判断 IP 地址：
//
//	addr, _ := netip.ParseAddr("192.168.1.1")
//	fmt.Println(xnet.AddrVersion(addr))       // IPv4
//	fmt.Println(xnet.FormatFullIPAddr(addr))  // 192.168.001.001
//
// 解析 IP 范围并判断包含关系：
//
//	r, _ := xnet.ParseRange("192.168.1.0/24")
//	addr := netip.MustParseAddr("192.168.1.100")
//	fmt.Println(r.Contains(addr))  // true
//
// 批量解析并合并重叠范围：
//
//	set, _ := xnet.ParseRanges([]string{
//	    "10.0.0.1-10.0.0.100",
//	    "10.0.0.50-10.0.0.150",  // 与上一个重叠
//	    "192.168.1.0/24",
//	})
//	fmt.Println(len(set.Ranges()))            // 2（重叠范围已合并）
//	fmt.Println(set.Contains(addr))           // O(log n) 高效查询
//
// 序列化 IP 范围：
//
//	r, _ := xnet.ParseRange("192.168.1.1-192.168.1.100")
//	w := xnet.WireRangeFrom(r)
//	data, _ := json.Marshal(w)
//	fmt.Println(string(data))  // {"s":"192.168.1.1","e":"192.168.1.100"}
//
// # 设计决策
//
//   - 直接使用 [netip.Addr] 值类型，零分配比较，可做 map key
//   - 使用 [netipx.IPRange] 和 [*netipx.IPSet]，无需自研集合与搜索逻辑
//   - [*netipx.IPSet] 提供 O(log n) 的高效范围查询
//   - [WireRange] 提供 JSON/BSON/YAML 序列化，字段格式 {"s":"start","e":"end"}
//   - 所有可失败函数返回 error，预定义错误变量支持 errors.Is
//   - 掩码格式解析（parseRangeWithMask）已包含连续性校验，拒绝非法掩码如 "255.0.255.0"
//
// # 地址转换
//
// IPv4 与 IPv6 映射转换：
//
//	addr := netip.MustParseAddr("192.168.1.1")
//	mapped := xnet.MapToIPv6(addr)           // ::ffff:192.168.1.1
//	unmapped := xnet.UnmapToIPv4(mapped)     // 192.168.1.1
//
// 地址加减运算：
//
//	addr := netip.MustParseAddr("192.168.1.100")
//	next, _ := xnet.AddrAdd(addr, 1)         // 192.168.1.101
//	prev, _ := xnet.AddrAdd(addr, -1)        // 192.168.1.99
//
// # 范围大小计算
//
// 计算 IP 范围包含的地址数量：
//
//	r, _ := xnet.ParseRange("192.168.1.0/24")
//	size := xnet.RangeSize(r)                // 256
//	sizeU64, _ := xnet.RangeSizeUint64(r)    // 256 (IPv4 优化版本)
//
// # 输入行为说明
//
// [ParseFullIP] 使用严格解析模式：
//   - 拒绝带有前导/尾随空白、+ 或 - 前缀的八位段
//   - 如 "1.2.3.+4"、"1.2.3. 4" 均返回解析错误
//   - 注：strconv.ParseUint 本身不接受 + 前缀，无需额外校验
//
// [FormatFullIPAddr] 和 [AddrToBigInt] 对无效地址返回零值：
//   - FormatFullIPAddr(netip.Addr{}) 返回空字符串 ""（与 netip.Addr.String 行为一致）
//   - AddrToBigInt(netip.Addr{}) 返回 big.Int 零值（便于链式调用）
//
// [MergeRanges] 返回错误而非静默返回 nil：
//   - 当输入包含无效范围（如 From > To）时返回 error
//   - 空切片返回 (nil, nil)
//
// # IPv4-mapped IPv6 地址处理
//
// [WireRangeFromAddrs] 将纯 IPv4 与 IPv4-mapped IPv6 视为不同族：
//   - 192.168.1.1（纯 IPv4）与 ::ffff:192.168.1.100（IPv4-mapped）不能混合
//   - 这确保序列化后的字符串格式一致，避免反序列化时产生歧义
//   - 如需跨族操作，请先使用 [MapToIPv6] 或 [UnmapToIPv4] 统一地址格式
//
// [AddrVersion] 将 IPv4-mapped IPv6 视为 V4（用于逻辑判断）：
//   - 这与 [WireRangeFromAddrs] 的"线路族"检查不同
//   - AddrVersion 关注语义版本，WireRangeFromAddrs 关注序列化一致性
//
// # IPSet 构建行为
//
// [IPSetFromRanges] 累积无效范围错误：
//   - netipx.IPSetBuilder.AddRange 累积错误，在 IPSet() 时返回
//   - 如需逐个校验范围，使用 [IPSetFromRangesStrict]
//
// # 错误处理
//
// 预定义错误变量支持 errors.Is 判断：
//
//	_, err := xnet.ParseRange("invalid")
//	if errors.Is(err, xnet.ErrInvalidRange) {
//	    // 处理无效范围
//	}
//
// # Go 版本要求
//
// xnet 要求 Go 1.23+（与 xmac 的 [iter.Seq] 依赖对齐）。
//
// 注意：Go 1.22.4 及更早版本的 [net/netip] 对 IPv4-mapped IPv6 地址
// 的分类存在 bug（详见 https://go.dev/issue/67289）。
// 当前最低要求 Go 1.23 已避开此问题。
//
// # 范围转 CIDR
//
// 将 IP 范围转换为 CIDR 前缀：
//
//	r, _ := xnet.ParseRange("192.168.1.0-192.168.1.255")
//	prefix, ok := xnet.RangeToPrefix(r)   // 192.168.1.0/24, true
//
//	r, _ = xnet.ParseRange("192.168.1.1-192.168.1.100")
//	prefixes := xnet.RangeToPrefixes(r)   // 分解为多个 CIDR 块
//
// # 从 gobase/mutils 迁移
//
// 以下是从 gobase mutils/iputils.go 迁移到 xnet 的 API 映射：
//
// IP 地址类型：
//
//	// gobase: 使用 MIP 结构体存储多种格式
//	mip, _ := mutils.NewMIP("192.168.1.1")
//	mip.StrIP     // 字符串
//	mip.NetIP     // net.IP
//	mip.IntIP     // uint32
//	mip.BigIntIP  // *big.Int
//
//	// xnet: 直接使用 netip.Addr 值类型
//	addr, _ := netip.ParseAddr("192.168.1.1")
//	addr.String()                 // 字符串
//	xnet.AddrToUint32(addr)       // uint32（按需转换）
//	xnet.AddrToBigInt(addr)       // *big.Int（按需转换）
//
// uint32 互转：
//
//	// gobase
//	mutils.StringToIPv4("192.168.1.1")  // → uint32
//	mutils.IPv4ToString(0xC0A80101)     // → string
//
//	// xnet
//	addr, _ := netip.ParseAddr("192.168.1.1")
//	v, _ := xnet.AddrToUint32(addr)           // → uint32
//	addr = xnet.AddrFromUint32(0xC0A80101)    // → netip.Addr
//
// FullIP 格式化：
//
//	// gobase
//	mutils.IP2FullIP("192.168.1.1")     // → "192.168.001.001"
//	mutils.FullIP2IP("192.168.001.001") // → "192.168.1.1"
//
//	// xnet
//	addr, _ := netip.ParseAddr("192.168.1.1")
//	xnet.FormatFullIPAddr(addr)               // → "192.168.001.001"
//	addr, _ = xnet.ParseFullIP("192.168.001.001")
//	addr.String()                             // → "192.168.1.1"
//
// IP 范围解析：
//
//	// gobase
//	ipr, _ := mutils.ParseIpRange("192.168.1.0/24")
//	ipr.BeginIp  // net.IP
//	ipr.EndIp    // net.IP
//
//	// xnet
//	r, _ := xnet.ParseRange("192.168.1.0/24")
//	r.From()     // netip.Addr
//	r.To()       // netip.Addr
//
// 范围包含判断：
//
//	// gobase: O(n) 线性搜索
//	ranges := mutils.IPRanges{...}
//	ranges.MustInit()
//	for _, r := range ranges {
//	    if r.Contains(mip) { ... }
//	}
//
//	// xnet: O(log n) 高效查询
//	set, _ := xnet.ParseRanges([]string{...})
//	set.Contains(addr)
//
// 范围合并：
//
//	// gobase
//	ranges.MergeAndSort()
//
//	// xnet
//	merged, _ := xnet.MergeRanges(ranges)
//
// 序列化结构：
//
//	// gobase: MIPRange 需要 MustInit() 初始化
//	mipr := mutils.MIPRange{S: "10.0.0.1", E: "10.0.0.100"}
//	mipr.MustInit()  // 必须调用
//
//	// xnet: WireRange 反序列化即可用
//	var w xnet.WireRange
//	json.Unmarshal(data, &w)
//	r, _ := w.ToIPRange()  // 直接转换
package xnet
