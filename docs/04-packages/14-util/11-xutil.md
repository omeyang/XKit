---
package: pkg/util/xutil
stability: Stable
coverage: 100.0%
tags: [util, generic]
---

# pkg/util/xutil

泛型小工具。当前仅 `If` 三目函数。

## 用途

- 简化某些三元表达（Go 没有三目运算符）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xutil"

x := xutil.If(cond, "yes", "no")
// 注意：trueVal 与 falseVal 都会被求值，没有短路！
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `If[T any](cond bool, trueVal, falseVal T) T` | 类型安全三目 |

## 设计要点

- **无短路**：Go 函数参数必先求值，不像 C `?:` 短路
- **简单优先**：刻意保持小，扩张需 ADR

## 相关

- API：[api.md#pkgutilxutil](../../03-conventions/01-api.md#pkgutilxutil)
