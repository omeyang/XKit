---
package: pkg/context/xenv
stability: Stable
coverage: 95.6%
tags: [context, env]
---

# pkg/context/xenv

环境变量读取与解析。带类型转换、默认值、必填校验。

## 用途

- 配置加载阶段读取环境变量
- 兼容 `os.Getenv` 但更安全（类型转换 / 默认 / 必填）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/context/xenv"

addr := xenv.String("LISTEN_ADDR", ":8080")
timeout := xenv.Duration("TIMEOUT", 30*time.Second)
debug := xenv.Bool("DEBUG", false)

token, err := xenv.Required("API_TOKEN")
if err != nil { /* missing */ }
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `String / Int / Bool / Duration / Float64` | 带默认值 |
| `Required(key) (string, error)` | 必填，缺失返错 |
| `MustRequired(key) string` | panic 版（启动期可用）|

## 设计要点

- **早失败**：`Required` 在启动期触发，远好过运行时空值
- **类型安全**：避免手动 `strconv.Atoi(os.Getenv(...))` 模板代码

## 相关

- API：[api.md#pkgcontextxenv](../../03-conventions/01-api.md#pkgcontextxenv)
