---
title: Lifecycle 包
tags: [moc, packages, lifecycle]
---

# Lifecycle 包

进程生命周期与健康探测。处理启动、关闭、健康检查的标准接口。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xrun](02-xrun.md) | 进程生命周期管理（errgroup + 信号处理） | Stable | 98.2% |
| [xhealth](01-xhealth.md) | Kubernetes 健康探针（liveness/readiness/startup） | Stable | 92.4% |

## 选型指南

| 场景 | 用 |
|---|---|
| 启动多个 goroutine 任务，统一管理生命周期 + 信号优雅退出 | [xrun](02-xrun.md) |
| 暴露 K8s `/healthz`、`/readyz` 端点 | [xhealth](01-xhealth.md) |

## 相关

- [Panic recover 纪律](../../05-concepts/08-panic-recover-discipline.md) - xhealth 的 statusListener 必须有 recover
- [API 清单 Lifecycle 段](../../03-conventions/01-api.md)
