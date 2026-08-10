---
title: Config 包
tags: [moc, packages, config]
---

# Config 包

配置管理。基于 koanf 的统一加载/合并/读取层。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xconf](01-xconf.md) | 配置管理（多源合并：文件 / 环境变量 / 远程） | Stable | 92.0% |

## 设计要点

- 后端：[koanf](https://github.com/knadh/koanf)
- 不引入 viper（依赖体积过大）
- 支持热加载（部分后端）

## 相关

- [API 清单](../../03-conventions/01-api.md#pkgconfigxconf)
