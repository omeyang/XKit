---
package: pkg/util/xjson
stability: Stable
coverage: 100.0%
tags: [util, json]
---

# pkg/util/xjson

JSON 格式化工具。重点是 `PrettyE` —— 用 defer/recover 拦截 `MarshalJSON` 内的 panic 转为错误。

## 用途

- 美化 JSON 输出（缩进、换行）
- 防御自定义 `MarshalJSON` panic 污染调用方

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xjson"

s, err := xjson.PrettyE(obj)
if err != nil {
    // 标准 marshal 错误,或 MarshalJSON panic 转化的 ErrMarshal
}
fmt.Println(s)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `PrettyE(v any) (string, error)` | 美化 + panic recover |
| `Pretty(v any) string` | 不报错版（panic → 空串）|
| `ErrMarshal` | panic 转化的错误（含 `%w` 保留底层 error 链）|

## 设计要点

- **`PrettyE` defer/recover**：拦截 `MarshalJSON` 自定义实现的 panic
- **`%w` 保留 panic error 链**：若 recover 的值实现 `error` 接口，用 `%w` 包装
- **Fuzz 测试**：覆盖空字符串、base64 等边界

## 相关

- 概念：
  - [Panic recover 纪律](../../05-concepts/08-panic-recover-discipline.md)
- API：[api.md#pkgutilxjson](../../03-conventions/01-api.md#pkgutilxjson)
