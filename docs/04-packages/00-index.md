---
title: 包索引
tags: [moc, packages]
---

# 包索引（MOC）

XKit 共 38 个包，按域分组。点击进入子域索引或直达包详情。

## 按域分组

| 域 | 包数 | 入口 |
|---|---|---|
| Business（业务） | 1 | [business/](01-business/00-index.md) |
| Config（配置） | 1 | [config/](02-config/00-index.md) |
| Context（上下文） | 4 | [context/](03-context/00-index.md) |
| Debug（调试） | 1 | [debug/](04-debug/00-index.md) |
| Distributed（分布式） | 4 | [distributed/](05-distributed/00-index.md) |
| Lifecycle（生命周期） | 2 | [lifecycle/](07-lifecycle/00-index.md) |
| MQ（消息队列） | 2 | [mq/](08-mq/00-index.md) |
| Observability（可观测性） | 5 | [observability/](09-observability/00-index.md) |
| Resilience（弹性） | 3 | [resilience/](10-resilience/00-index.md) |
| Security（安全） | 1 | [security/](11-security/00-index.md) |
| Storage（存储） | 4 | [storage/](12-storage/00-index.md) |
| Testkit（测试辅助） | 2 | [testkit/](13-testkit/00-index.md) |
| Util（通用工具） | 11 | [util/](14-util/00-index.md) |
| Internal（仅内部） | 4 | [internal/](06-internal/00-index.md) |

## 按稳定性分组

- **Stable**（API 冻结，向前兼容）：xctx, xtenant, xplatform, xenv, xlog, xtrace, xmetrics, xrotate, xcache, xrun, xfile, xjson, xlru, xpool, xproc, xsys, xutil
- **Beta**（功能稳定，API 待冻结）：xauth, xconf, xdbg, xtls, xetcd, xmongo, xclickhouse, xdlock, xcron, xelection, xsemaphore, xkafka, xpulsar, xbreaker, xretry, xlimit, xhealth, xid, xkeylock, xmac, xnet
- **Alpha**（设计可能调整）：xsampling
- **Internal**（不对外，测试或共享逻辑）：xetcdtest, xredismock, xsemaphoremock, deploy, mqcore, rediscompat, storageopt

## 按覆盖率（实测）

完整数据见 [进度追踪](../02-progress.md)。整体 94.0%；以下为 100% 覆盖包：

- `pkg/observability/xmetrics`、`pkg/context/xplatform`
- `pkg/util/xjson`、`xkeylock`、`xlru`、`xpool`、`xproc`、`xutil`
- `internal/deploy`、`mqcore`、`rediscompat`、`storageopt`

## 文档约定

每个包详情页（`*.md`）包含：

1. **YAML frontmatter**：稳定性、覆盖率、tags、relations
2. **用途**：定位 + 适用 / 不适用场景
3. **快速上手**：最小可运行示例
4. **关键类型与函数**：表格列出公开 API
5. **设计要点**：决策与陷阱（链接 ADR / concepts / patterns）
6. **相关**：跨链接到 concepts、patterns、ADR、api.md 锚点

## 相关

- [概念笔记 MOC](../05-concepts/00-index.md)
- [设计模式 MOC](../06-patterns/00-index.md)
- [术语表](../07-glossary.md)
- [API 清单](../03-conventions/01-api.md)
