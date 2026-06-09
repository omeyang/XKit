---
title: Util 包
tags: [moc, packages, util]
---

# Util 包

通用工具集。按 [ADR-0007](../../01-decisions/0007-util-packages-no-builtin-observability.md)：**util 包不内置可观测性**（不引入 OTel/logger 依赖）。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xfile](01-xfile.md) | 文件操作（路径安全） | Stable | 96.9% |
| [xid](02-xid.md) | Sonyflake v2 分布式 ID 生成 | Beta | 99.1% |
| [xjson](03-xjson.md) | JSON 格式化（PrettyE 等） | Stable | 100.0% |
| [xkeylock](04-xkeylock.md) | 基于 key 的进程内互斥锁（分片 channel） | Beta | 100.0% |
| [xlru](05-xlru.md) | LRU 缓存（泛型 + TTL） | Stable | 100.0% |
| [xmac](06-xmac.md) | MAC 地址工具（多格式解析、`iter.Seq` 迭代） | Beta | 98.2% |
| [xnet](07-xnet.md) | IP 地址工具（net/netip + WireRange） | Beta | 98.4% |
| [xpool](08-xpool.md) | 泛型 Worker Pool | Stable | 100.0% |
| [xproc](09-xproc.md) | 进程信息查询（PID / 名称） | Stable | 100.0% |
| [xsys](10-xsys.md) | 系统资源限制（RLIMIT_NOFILE） | Stable | 96.0% |
| [xutil](11-xutil.md) | 泛型工具（If 三目运算等） | Stable | 100.0% |

## 选型指南

| 场景 | 用 |
|---|---|
| 生成唯一 ID（分布式 + 时序） | [xid](02-xid.md) |
| 按 key 互斥（同 key 互斥，不同 key 并发） | [xkeylock](04-xkeylock.md) |
| 进程内泛型缓存 | [xlru](05-xlru.md) |
| 并发任务池 | [xpool](08-xpool.md) |
| 解析 IP/CIDR/范围 | [xnet](07-xnet.md) |
| 解析 MAC 地址或区间迭代 | [xmac](06-xmac.md) |

## 相关 ADR

- [0007 util 包不内置可观测性](../../01-decisions/0007-util-packages-no-builtin-observability.md)
