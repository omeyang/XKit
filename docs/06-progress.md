# 06 · 进度追踪

只记录**当前状态**。历史发布记录见 `CHANGELOG.md`。

## 包稳定性矩阵

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/context/xctx` | Stable | 97.7% | |
| `pkg/context/xtenant` | Stable | — | |
| `pkg/context/xplatform` | Stable | — | |
| `pkg/context/xenv` | Stable | — | |
| `pkg/observability/xlog` | Stable | — | |
| `pkg/observability/xtrace` | Stable | — | W3C Trace Context |
| `pkg/observability/xmetrics` | Stable | — | OTel 抽象 |
| `pkg/observability/xrotate` | Stable | — | 基于 lumberjack |
| `pkg/observability/xsampling` | Alpha | — | 采样策略可能调整 |
| `pkg/resilience/xbreaker` | Beta | — | |
| `pkg/resilience/xretry` | Beta | — | |
| `pkg/resilience/xlimit` | Beta | — | |
| `pkg/storage/xcache` | Stable | — | |
| `pkg/storage/xetcd` | Beta | — | |
| `pkg/storage/xmongo` | Beta | — | |
| `pkg/storage/xclickhouse` | Beta | — | |
| `pkg/distributed/xdlock` | Beta | — | |
| `pkg/distributed/xcron` | Beta | — | |
| `pkg/mq/xkafka` | Beta | — | DLQ + OTel |
| `pkg/mq/xpulsar` | Beta | — | DLQ + OTel |
| `pkg/config/xconf` | Beta | — | koanf |
| `pkg/business/xauth` | Beta | — | 双层缓存 |
| `pkg/debug/xdbg` | Beta | — | Unix Socket |
| `pkg/lifecycle/xrun` | Stable | — | errgroup + signal |
| `pkg/util/xfile` | Stable | — | |
| `pkg/util/xjson` | Stable | 100% | |
| `pkg/util/xkeylock` | Beta | 100% | |
| `pkg/util/xlru` | Stable | 100% | |
| `pkg/util/xmac` | Beta | 99.0% | |
| `pkg/util/xnet` | Beta | 98.4% | |
| `pkg/util/xpool` | Stable | 100% | |
| `pkg/util/xproc` | Stable | — | |
| `pkg/util/xsys` | Stable | Linux 96.0% / BSD 100% | 跨平台 build tag |
| `pkg/util/xutil` | Stable | 100% | |
| `internal/xsemaphore` | Internal | 93.3% | Redis 分布式信号量 |
| `internal/xid` | Internal | 99.1% | Sonyflake v2 |

> 覆盖率以 memory 记录为准，权威数字以 `task test-cover` 输出为准。

## 进行中

（无已承诺的跨版本大型重构。新功能按 `docs/07-conventions/contributing.md` 的规范驱动流程推进。）

## 待补齐

- `README.md` 包覆盖率列未填项需补基准数据
- 部分 Beta 包升级至 Stable 需要：API 冻结窗口 + 真实业务回归
