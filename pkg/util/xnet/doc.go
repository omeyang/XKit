// Package xnet 提供 IP 地址工具库。
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
//   - wire.go: [WireRange] JSON/BSON 序列化的 IP 范围结构
//
// # 设计决策
//
//   - 直接使用 [net/netip.Addr] 值类型，零分配比较，可做 map key
//   - 使用 [go4.org/netipx.IPRange] 和 [*netipx.IPSet]，无需自研集合与搜索逻辑
//   - [WireRange] 提供 JSON/BSON 序列化，字段格式 {"s":"start","e":"end"}
//   - 所有可失败函数返回 error，预定义错误变量支持 errors.Is
//   - 掩码格式解析（parseRangeWithMask）已包含连续性校验（inverted & (inverted+1) != 0），
//     拒绝非法掩码如 "255.0.255.0"
package xnet
