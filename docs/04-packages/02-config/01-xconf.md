---
package: pkg/config/xconf
stability: Stable
coverage: 92.0%
tags: [config, koanf]
---

# pkg/config/xconf

配置管理。基于 [koanf](https://github.com/knadh/koanf)，支持文件 / 环境变量 / 命令行 / 远程多源合并。

## 用途

- 应用启动加载配置（YAML/TOML/JSON）
- 多环境覆盖（base + override）
- 热加载（部分后端）

## 不适用场景

- 不引入 viper（依赖太重）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/config/xconf"

c, err := xconf.New(
    xconf.WithFile("config.yaml"),
    xconf.WithEnv("MYAPP_"),
)
if err != nil { /* ... */ }

var cfg struct {
    Listen string
    DB     struct{ DSN string }
}
_ = c.Unmarshal("", &cfg)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Config` interface | `Client() *koanf.Koanf / Unmarshal(path, target)` |
| `New(opts...) (Config, error)` | 入口 |
| `WithFile / WithEnv / WithRemote` | 后端注入 |

## 设计要点

- **`Reset` / `Init` / `InitWith` 顺序保证**：先写值再标志，避免读到未初始化状态
- **`dt==""`** 注释明确允许（doc.go:L231-233）

## 相关

- 包：[xenv](../03-context/02-xenv.md)（直接读 ENV 的简化场景）
- API：[api.md#pkgconfigxconf](../../03-conventions/01-api.md#pkgconfigxconf)
