---
package: pkg/util/xnet
stability: Stable
coverage: 98.4%
tags: [util, net, ip, netip]
related:
  - ../../05-concepts/02-error-handling-policy.md
---

# pkg/util/xnet

IP 地址工具。基于 `net/netip` + `go4.org/netipx`，提供 IP/CIDR/范围解析、分类、序列化与高性能转换。

## 用途

- 解析 IP / CIDR / 掩码 / 范围（`10.0.0.0/24`、`10.0.0.1-10.0.0.5`）
- IP 分类（私有 / 链路本地 / 组播 / 广播 / 文档保留等）
- 高效 IP↔uint32/big.Int 转换
- 范围序列化（`WireRange` JSON/BSON/YAML 互通）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xnet"

// 解析
r, _ := xnet.ParseRange("192.168.1.0/24")
size := xnet.RangeSizeUint64(r)

// 分类
cls := xnet.Classify(netip.MustParseAddr("10.0.0.1"))
fmt.Println(cls.IsPrivate, cls.String()) // true, "private"

// 批量集合
set, _ := xnet.ParseRanges([]string{"10.0.0.0/8", "192.168.0.0/16"})
fmt.Println(set.Contains(netip.MustParseAddr("10.1.1.1"))) // true
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `ParseRange / ParseRanges` | 解析 IP/CIDR/掩码/范围 |
| `Classify(addr) Classification` | 15 字段分类（私有/链路本地/组播/广播/Benchmark/Documentation 等）|
| `WireRange` | JSON 友好范围（`{"start":"...","end":"..."}`，含 `IsZero()`）|
| `AddrFromUint32 / AddrToUint32` | IPv4 ↔ uint32 |
| `AddrFromBigInt / AddrToBigInt` | IP ↔ big.Int |
| `FormatFullIP / ParseFullIP` | 全长格式化（IPv4 `192.168.001.001`、IPv6 32 hex）|
| `RangeSize / RangeSizeUint64` | 范围大小（IPv4 快路径 1 alloc）|
| `MergeRanges` | 合并重叠/相邻范围 |

## 设计要点

- **`unmapAddr`** 在所有 ParseRange 路径里归一化 IPv4-mapped IPv6 → 纯 IPv4
- **错误链**：跨包用 `%w` 包装 `ParseAddr` 底层错（FG-M fix）
- **`FormatFullIPAddr` IPv6 路径**：`hex.Encode` + 栈 `[32]byte` 缓冲，1 alloc vs 上一版的 2
- **`IsBroadcast`** 优先级在 `IsMulticast` 之前；`255.255.255.255` 是 broadcast 不是 reserved
- **`IsBenchmark`** 覆盖 IPv4（`198.18.0.0/15`）+ IPv6（`2001:2::/48` RFC 5180）
- **`WireRangeFromAddrs`** 拒绝带 zone ID 的地址（避免 round-trip 破损）

## 相关

- 概念：
  - [错误处理策略](../../05-concepts/02-error-handling-policy.md)
- API：[api.md#pkgutilxnet](../../03-conventions/01-api.md#pkgutilxnet)
