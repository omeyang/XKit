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
//   - contains.go: IP 范围包含判断、合并、大小计算、CIDR 转换等
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
//	w, _ := xnet.WireRangeFrom(r)
//	data, _ := json.Marshal(w)
//	fmt.Println(string(data))  // {"start":"192.168.1.1","end":"192.168.1.100"}
//
// # 设计决策
//
//   - 直接使用 [netip.Addr] 值类型，零分配比较，可做 map key
//   - 使用 [netipx.IPRange] 和 [*netipx.IPSet]，无需自研集合与搜索逻辑
//   - [*netipx.IPSet] 提供 O(log n) 的高效范围查询
//   - [WireRange] 提供 JSON/BSON/YAML 序列化，字段格式 {"start":"...","end":"..."}
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
// # IPv6 Zone ID 处理
//
// [ParseRange]、[ParseRanges] 和 [WireRange.ToIPRange] 拒绝包含 IPv6 zone ID
// 的地址（如 "fe80::1%eth0"），返回 [ErrInvalidRange] 或 [ErrInvalidAddress]。
// 原因：[netipx.IPRange] 和 [*netipx.IPSet] 会静默丢弃 zone 信息，
// 导致后续查询不匹配（ACL/白名单/黑名单误判）。
//
// # IP 地址分类
//
// [Classify] 返回地址的各种分类信息，包括 [IsReserved]（240.0.0.0/4, Class E）。
// 分类标志不互斥，例如 240.0.0.1 同时满足 IsGlobalUnicast 和 IsReserved。
// [Classification.String] 按优先级返回最特殊的分类标签。
// [IsBenchmark] 同时覆盖 IPv4 (198.18.0.0/15, RFC 2544) 和 IPv6 (2001:2::/48, RFC 5180)。
// [IsSharedAddress]、[IsReserved] 和 [IsBroadcast] 仅适用于 IPv4（无对应 IPv6 范围）。
// [IsReserved] 排除 255.255.255.255（有限广播地址），使用 [IsBroadcast] 判断广播地址。
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
// [ParseRange] 对所有语法的 IPv4-mapped IPv6 地址统一归一化为纯 IPv4：
//   - 单 IP: "::ffff:192.168.1.1" → 纯 IPv4 范围 192.168.1.1-192.168.1.1
//   - CIDR: "::ffff:192.168.1.0/120" → 纯 IPv4 /24（bits ≥ 96 时转换，< 96 时拒绝）
//   - 掩码: "::ffff:192.168.1.0/255.255.255.0" → 纯 IPv4 范围
//   - 显式范围: "::ffff:192.168.1.1-::ffff:192.168.1.100" → 纯 IPv4 范围
//   - 这确保四种格式的输出地址族一致，避免规则集合中的匹配偏差
//
// [ParseFullIP] 接受 IPv4-mapped IPv6 地址（如 "::ffff:192.168.1.1"）：
//   - 此格式的点分特征会触发 IPv4 解析尝试，但首段含 ":" 导致失败
//   - 失败后自动回退到标准 [netip.ParseAddr]，正确返回 IPv4-mapped 地址
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
// [IPSetFromRanges] 累积无效范围错误，统一包装为 [ErrInvalidRange]：
//   - netipx.IPSetBuilder.AddRange 累积错误，在 IPSet() 时返回，外层包装 [ErrInvalidRange]
//   - errors.Is(err, [ErrInvalidRange]) 可用于统一错误分流
//   - 如需逐个校验范围并获得具体索引信息，使用 [IPSetFromRangesStrict]
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
// xnet 要求 Go 1.25+（与项目 go.mod 对齐）。
//
// 注意：Go 1.22.4 及更早版本的 [net/netip] 对 IPv4-mapped IPv6 地址
// 的分类存在 bug（详见 https://go.dev/issue/67289）。
// 此 bug 在 Go 1.23 中已修复，项目当前要求 Go 1.25+ 不受影响。
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
// gobase mutils/iputils.go → xnet 的核心 API 映射：
//
//	MIP 结构体        → netip.Addr 值类型（按需调用 AddrToUint32/AddrToBigInt 转换）
//	StringToIPv4      → netip.ParseAddr + AddrToUint32
//	IPv4ToString      → AddrFromUint32 + .String()
//	IP2FullIP         → FormatFullIPAddr
//	FullIP2IP         → ParseFullIP
//	ParseIpRange      → ParseRange（返回 netipx.IPRange）
//	IPRanges.Contains → ParseRanges 构建 *netipx.IPSet，O(log n) 查询
//	MergeAndSort      → MergeRanges
//	MIPRange{S,E}     → WireRange{Start,End}（反序列化即可用，无需 MustInit）
package xnet
