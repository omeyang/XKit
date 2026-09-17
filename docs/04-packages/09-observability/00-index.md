---
title: Observability 包
tags: [moc, packages, observability]
---

# Observability 包

可观测性三件套：日志 / 链路 / 指标，加上采样与轮转。统一使用 `log/slog` + OpenTelemetry。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xlog](01-xlog.md) | 结构化日志（slog 后端，唯一日志栈） | Stable | 96.8% |
| [xtrace](05-xtrace.md) | 链路追踪中间件（W3C Trace Context） | Stable | 92.6% |
| [xmetrics](02-xmetrics.md) | 统一可观测性接口（OTel 抽象） | Stable | 100.0% |
| [xrotate](03-xrotate.md) | 日志轮转（基于 lumberjack） | Stable | 96.9% |
| [xsampling](04-xsampling.md) | 采样策略（比例 / 错误偏置 / 组合） | Stable | 97.9% |

## 设计原则

- 日志栈：**只有 `log/slog`**，禁止引入 zap/zerolog/logrus（见 [ADR-0005](../../01-decisions/0005-slog-as-sole-log-backend.md)）
- 链路：W3C Trace Context 协议（跨语言/平台标准）
- 指标：OTel Meter，vendor-neutral，可对接 Prometheus / Jaeger / Tempo

## 相关概念

- [可观测性栈](../../05-concepts/07-observability-stack.md)
- [Typed-nil 陷阱](../../05-concepts/10-typed-nil-trap.md) - xmetrics 历史 Bug

## 相关 ADR

- [0005 slog 作为唯一日志后端](../../01-decisions/0005-slog-as-sole-log-backend.md)
- [0007 util 包不内置可观测性](../../01-decisions/0007-util-packages-no-builtin-observability.md)

## 相关 API

- [API 清单 Observability 段](../../03-conventions/01-api.md)
