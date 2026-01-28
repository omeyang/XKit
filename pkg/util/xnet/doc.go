// Package xnet 提供 IP 地址工具库，替代 gobase mbase/mutils 中所有 IP 相关功能。
//
// xnet 基于 Go 标准库 [net/netip] 和社区库 [go4.org/netipx] 构建，
// 直接使用 [net/netip.Addr] 和 [go4.org/netipx.IPRange] / [go4.org/netipx.IPSet]，
// 提供 IP 版本转换、格式化、解析和序列化等增量工具函数。
//
// # 核心功能
//
//   - convert.go: IP 版本判断、uint32/BigInt 与 [netip.Addr] 互转
//   - format.go: FullIP 全长格式化（"192.168.001.001"）、标准化、校验
//   - parse.go: 解析单 IP/CIDR/掩码/范围格式为 [netipx.IPRange]，批量解析为 [*netipx.IPSet]
//   - wire.go: [WireRange] JSON/BSON 序列化兼容 gobase MIPRange 格式
//
// # 与 gobase 迁移对照
//
//	gobase                    → xnet / 标准库
//	───────────────────────────────────────────────────────
//	mutils.NewMIP(s)          → netip.ParseAddr(s)
//	mutils.NewMIPRange(s, e)  → netipx.IPRangeFrom(s, e)
//	mutils.IPRanges           → *netipx.IPSet
//	mutils.BinarySearchIP     → set.Contains(addr)
//	mutils.IP2FullIP(s)       → xnet.FormatFullIP(s)
//	mutils.FullIP2IP(s)       → xnet.ParseFullIP(s)
//	mutils.IsValidIP(s)       → xnet.IsValidIP(s)
//	mutils.MergeAndSort()     → 自动（IPSet 始终合并）
//
// # 旧 xnet → 新 xnet 迁移速查
//
//	旧 xnet                       → 新
//	────────────────────────────────────────────────────
//	Address                       → netip.Addr
//	ParseAddress(s)               → netip.ParseAddr(s)
//	MustParseAddress(s)           → netip.MustParseAddr(s)
//	Address.Compare()             → addr.Compare()
//	Address.IsValid()             → addr.IsValid()
//	Address.String()              → addr.String()
//	Address.FullIP()              → xnet.FormatFullIPAddr(addr)
//	Address.As4Uint32()           → xnet.AddrToUint32(addr)
//	Address.AsBigInt()            → xnet.AddrToBigInt(addr)
//	Address.Version()             → xnet.AddrVersion(addr)
//	AddressFromUint32(v)          → xnet.AddrFromUint32(v)
//	AddressFromBigInt(v,ver)      → xnet.AddrFromBigInt(v,ver)
//	Range                         → netipx.IPRange
//	NewRange(s, e)                → netipx.IPRangeFrom(s, e)
//	ParseRange(s)                 → xnet.ParseRange(s)
//	Range.Start()                 → r.From()
//	Range.End()                   → r.To()
//	Range.Contains(a)             → r.Contains(a)
//	Range.MarshalJSON()           → json.Marshal(xnet.WireRangeFrom(r))
//	Ranges                        → *netipx.IPSet
//	ParseRanges(strs)             → xnet.ParseRanges(strs)
//	Ranges.Contains(a)            → set.Contains(a)
//	Ranges.MergeAndSort()         → 自动（IPSet 始终合并）
//	Ranges.Items()                → set.Ranges()
//	BinarySearch[T]               → set.Contains(addr)
//	Searchable                    → 删除（不再需要）
//
// # 基本用法
//
//	addr, err := netip.ParseAddr("192.168.1.1")
//	if err != nil {
//	    return err
//	}
//
//	r, err := xnet.ParseRange("192.168.1.0/24")
//	if err != nil {
//	    return err
//	}
//	fmt.Println(r.Contains(addr)) // true
//
//	set, err := xnet.ParseRanges([]string{"192.168.1.0/24", "10.0.0.0/8"})
//	if err != nil {
//	    return err
//	}
//	fmt.Println(set.Contains(addr)) // true
//
// # 设计决策
//
//   - 直接使用 [net/netip.Addr] 值类型，零分配比较，可做 map key
//   - 使用 [go4.org/netipx.IPRange] 和 [*netipx.IPSet] 替代自研 Range/Ranges/BinarySearch
//   - [WireRange] 提供 JSON/BSON 序列化兼容 gobase MIPRange 格式
//   - 所有可失败函数返回 error，预定义错误变量支持 errors.Is
//   - 掩码格式解析（parseRangeWithMask）已包含连续性校验（inverted & (inverted+1) != 0），
//     拒绝非法掩码如 "255.0.255.0"
package xnet
