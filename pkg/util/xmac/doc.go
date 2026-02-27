// Package xmac 提供 MAC 地址处理工具。
//
// xmac 基于 Go 标准库 [net] 构建，提供类型安全的 MAC 地址操作：
//
//   - 多格式解析（冒号、短线、点、无分隔符）
//   - 多格式输出（FormatColon, FormatDash, FormatDot, FormatBare 及对应 Upper 变体）
//   - 地址属性判断（单播/多播、本地/全局管理）
//   - JSON/Text/Binary/SQL 序列化支持
//   - 地址运算（Next/Prev）
//
// # 快速示例
//
// 解析和格式化：
//
//	addr, err := xmac.Parse("AA:BB:CC:DD:EE:FF")
//	fmt.Println(addr.String())                 // aa:bb:cc:dd:ee:ff
//	fmt.Println(addr.FormatString(xmac.FormatDash))  // aa-bb-cc-dd-ee-ff
//
// 验证地址类型：
//
//	if addr.IsUsable() {
//	    // 有效且可用于业务
//	}
//	if addr.IsUnicast() {
//	    // 单播地址
//	}
//
// JSON 序列化：
//
//	type Asset struct {
//	    MAC xmac.Addr `json:"mac"`
//	}
//	json.Marshal(Asset{MAC: addr})  // {"mac":"aa:bb:cc:dd:ee:ff"}
//
// # 设计决策
//
//   - 使用 [6]byte 固定数组而非 []byte 切片：值语义、可比较、栈分配
//   - 仅支持 EUI-48 (6字节)，不支持 EUI-64 (8字节)
//   - 内部统一小写存储，输出格式可选
//   - 零值表示无效地址，受 [net/netip.Addr] 零值语义启发（详见下方"零值与有效性语义"）
//   - JSON 序列化：无效地址输出 ""（空字符串），保证 JSON 往返一致性；
//     SQL 序列化：无效地址输出 nil（SQL NULL）。如需 JSON null，使用指针类型 *Addr
//
// # 零值与有效性语义
//
// xmac 区分两种"有效性"概念：
//
// 数学有效性 [Addr.IsValid]：
//   - 零值 Addr{} 返回 false，表示变量未初始化
//   - [Parse] 函数对全零 MAC 地址 "00:00:00:00:00:00" 返回零值 Addr{}
//   - 设计决策: 零值无效的概念受 [net/netip.Addr] 启发。与 [net/netip.Addr] 不同，
//     xmac 无法区分未初始化零值与已解析的全零 MAC——两者均为 Addr{}。
//     这是 [6]byte 值类型的固有限制（无额外标志位），且全零 MAC 在实际网络中
//     无业务意义，因此不影响实际使用
//
// 业务可用性 [Addr.IsUsable]：
//   - 排除特殊地址（全零、广播）
//   - 资产识别等业务场景应使用此方法
//
// 示例：
//
//	var uninit xmac.Addr     // 零值
//	uninit.IsValid()         // false - 未初始化
//	uninit.String()          // "" - 无效地址返回空字符串
//
//	zero, _ := xmac.Parse("00:00:00:00:00:00")
//	zero == uninit           // true - 全零地址等于零值
//	zero.IsValid()           // false
//	zero.IsUsable()          // false
//
// # 迭代器与零值
//
// [Range]、[RangeN]、[RangeWithIndex]、[RangeReverse] 等数学操作接受任何地址值（包括零值），
// 因为它们是对 48 位地址空间的数学运算，而非业务有效性判断：
//
//	// Range 接受零地址参与迭代（视为 00:00:00:00:00:00）
//	for addr := range xmac.Range(xmac.Addr{}, xmac.MustParse("00:00:00:00:00:02")) {
//	    fmt.Println(addr)  // 输出 3 个地址: 00, 01, 02
//	}
//
//	// RangeReverse 反向迭代
//	for addr := range xmac.RangeReverse(xmac.MustParse("00:00:00:00:00:01"), xmac.MustParse("00:00:00:00:00:03")) {
//	    fmt.Println(addr)  // 输出: 03, 02, 01
//	}
//
//	// [RangeCount] 同理
//	xmac.RangeCount(xmac.Addr{}, xmac.Addr{})  // 返回 1
//
// # 错误处理
//
// 预定义错误变量支持 errors.Is 判断：
//
//	addr, err := xmac.Parse("invalid")
//	if errors.Is(err, xmac.ErrInvalidFormat) {
//	    // 格式错误
//	}
//
// # 平台要求
//
// xmac 使用 Go 1.25+ 的 [iter.Seq] 迭代器特性。
// 最低要求 Go 1.25（与项目 go.mod 一致）。
//
// 设计决策: 迭代器索引（[RangeWithIndex]、[RangeReverseWithIndex]）和 [Count] 使用 int 类型，
// 与 Go 标准库 [slices.All] 保持一致。在 64 位架构上 int 为 64 位，可覆盖完整的 48 位
// MAC 地址空间。32 位架构上超过 math.MaxInt32 的范围会溢出——如需处理超大范围，
// 请使用 [RangeCount]（返回 uint64）。
package xmac
