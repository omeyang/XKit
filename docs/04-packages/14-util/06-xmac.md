---
package: pkg/util/xmac
stability: Beta
coverage: 98.2%
tags: [util, mac, iter]
---

# pkg/util/xmac

MAC 地址工具。多格式解析、Go 1.23+ `iter.Seq` 区间迭代、批量收集。

## 用途

- 解析多种 MAC 格式（`aa:bb:cc:dd:ee:ff` / `aa-bb-...` / `aabb.ccdd.eeff` / 裸 hex）
- MAC 范围迭代（生成连续 N 个、或两 MAC 之间）
- 批量收集到 slice

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xmac"

m, _ := xmac.Parse("aa:bb:cc:dd:ee:ff")
fmt.Println(m.String())

// Go 1.23+ iter
for addr := range xmac.RangeN(m, 100) {
    process(addr)
}

list := xmac.CollectN(xmac.RangeN(m, 10), 10)
total := xmac.RangeCount(start, end)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Addr` | 6 字节 MAC，值类型 |
| `Format uint8` | 序列化格式（Canonical/Cisco/Bare 等）|
| `Parse / MustParse / ParseBytes / FromHardwareAddr` | 解析 |
| `Zero / Broadcast` | 特殊地址 |
| `Range / RangeN / RangeWithIndex / RangeReverse / RangeReverseWithIndex` | `iter.Seq` / `iter.Seq2` 迭代 |
| `CollectN / Count / RangeCount` | 聚合 |
| `AddrToUint64 / Uint64ToAddr` | uint64 互转 |
| `AddrFrom6([6]byte)` | 字节数组构造 |

## 设计要点

- **`iter.Seq` 优先**：Go 1.23+ 标准，可与 `slices` 包组合
- **`uint64` 转换**：方便排序、做哈希 key
- **Format 枚举**：避免字符串魔法值

## 相关

- API：[api.md#pkgutilxmac](../../03-conventions/01-api.md#pkgutilxmac)
