# 06 · 进度追踪

只记录**当前状态**。历史发布记录见 `CHANGELOG.md`。

## 包稳定性矩阵

> 覆盖率数据来源：`task test-cover`（语句级加权）。整体 94.0%；核心包均 ≥ 90%。

### Context（上下文与身份管理）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/context/xctx` | Stable | 97.7% | |
| `pkg/context/xtenant` | Stable | 96.8% | HTTP/gRPC 双协议中间件 |
| `pkg/context/xplatform` | Stable | 100.0% | |
| `pkg/context/xenv` | Stable | 95.6% | |

### Observability（可观测性）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/observability/xlog` | Stable | 96.8% | |
| `pkg/observability/xtrace` | Stable | 92.6% | W3C Trace Context |
| `pkg/observability/xmetrics` | Stable | 100.0% | OTel 抽象 |
| `pkg/observability/xrotate` | Stable | 96.9% | 基于 lumberjack |
| `pkg/observability/xsampling` | Alpha | 97.9% | 采样策略可能调整 |

### Resilience（弹性与容错）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/resilience/xbreaker` | Beta | 99.4% | |
| `pkg/resilience/xretry` | Beta | 96.3% | |
| `pkg/resilience/xlimit` | Beta | 95.3% | 分布式限流 Token Bucket |

### Storage（数据存储）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/storage/xcache` | Stable | 93.8% | |
| `pkg/storage/xetcd` | Beta | 94.3% | 含 Informer list+watch 缓存 |
| `pkg/storage/xmongo` | Beta | 96.0% | |
| `pkg/storage/xclickhouse` | Beta | 96.7% | 批次原子性 + COUNT UInt64 |

### Distributed（分布式协调）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/distributed/xdlock` | Beta | 94.6% | |
| `pkg/distributed/xcron` | Beta | 94.1% | |
| `pkg/distributed/xelection` | Beta | 95.4% | 基于 etcd concurrency |
| `pkg/distributed/xsemaphore` | Beta | 94.1% | Redis Lua + Fallback；多轮对抗审查稳定 |

### MQ（消息队列）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/mq/xkafka` | Beta | 88.5% | DLQ + OTel |
| `pkg/mq/xpulsar` | Beta | 99.5% | DLQ + OTel |

### Config / Business / Debug / Security

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/config/xconf` | Beta | 92.0% | koanf |
| `pkg/business/xauth` | Beta | 95.0% | 双层缓存 |
| `pkg/debug/xdbg` | Beta | 90.5% | Unix Socket |
| `pkg/security/xtls` | Beta | 90.5% | TLS 配置 + 证书加载 |

### Lifecycle（进程生命周期）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/lifecycle/xrun` | Stable | 98.2% | errgroup + signal |
| `pkg/lifecycle/xhealth` | Beta | 92.4% | K8s liveness/readiness/startup |

### Util（通用工具）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/util/xfile` | Stable | 96.9% | |
| `pkg/util/xid` | Beta | 99.1% | Sonyflake v2；多轮对抗审查稳定 |
| `pkg/util/xjson` | Stable | 100.0% | |
| `pkg/util/xkeylock` | Beta | 100.0% | |
| `pkg/util/xlru` | Stable | 100.0% | |
| `pkg/util/xmac` | Beta | 98.2% | |
| `pkg/util/xnet` | Beta | 98.4% | |
| `pkg/util/xpool` | Stable | 100.0% | |
| `pkg/util/xproc` | Stable | 100.0% | |
| `pkg/util/xsys` | Stable | 96.0% | 跨平台 build tag |
| `pkg/util/xutil` | Stable | 100.0% | |

### Testkit（测试辅助，不对生产使用）

| 包 | 稳定性 | 覆盖率 | 备注 |
|---|---|---|---|
| `pkg/testkit/xetcdtest` | Internal | 70.5% | 嵌入式 etcd server 用于集成测试 |
| `pkg/testkit/xredismock` | Internal | 87.0% | Redis Mock 客户端 |
| `pkg/distributed/xsemaphore/xsemaphoremock` | Internal | — | mockgen 生成的 gomock 桩（无自身测试，由调用方覆盖） |

### Internal（仅限项目内部使用）

| 包 | 覆盖率 | 用途 |
|---|---|---|
| `internal/deploy` | 100.0% | 部署相关共享逻辑 |
| `internal/mqcore` | 100.0% | MQ 通用消费循环（xkafka/xpulsar 共享，fail-fast 设计） |
| `internal/rediscompat` | 100.0% | Redis 代理脚本模式探测（DetectScriptMode / Bounded） |
| `internal/storageopt` | 100.0% | 存储层共享选项 |

### CLI 工具

| 路径 | 覆盖率 | 用途 |
|---|---|---|
| `cmd/xdbgctl` | 91.1% | xdbg 调试服务的命令行客户端 |

## 进行中

（无已承诺的跨版本大型重构。新功能按 `docs/03-conventions/02-contributing.md` 的规范驱动流程推进。）

## 待补齐

- 部分 Beta 包升级至 Stable 需要：API 冻结窗口 + 真实业务回归
- `pkg/observability/xsampling` 由 Alpha 升 Beta 需要：采样策略稳定性验证
- `pkg/mq/xkafka` 覆盖率 88.5% 低于整体 90% 目标线，需补充错误路径用例
- `pkg/testkit/xetcdtest` 覆盖率 70.5%，集成测试桩本身的测试不完整
