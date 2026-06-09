# XKit

XKit 是独立的 Go 基础工具库，为业务开发提供通用功能支持。文档只关注当下可用能力与使用方式，不引入历史演变。

> AI/RAG 全文喂入：`llms-full.txt` 由 CI 在 push/tag 时生成（Actions 工作流产物 + 在 tag 上附加到 GitHub Release）；本地可运行 `task docs-llms` 按需生成，文件已加入 `.gitignore`。

---

## 核心特性

- **模块化设计**：每个功能独立、可复用，依赖关系清晰
- **API 稳定性**：遵循语义化版本控制
- **测试优先**：TDD 开发模式，核心业务测试覆盖率 ≥ 95%，整体 ≥ 90%
- **代码质量**：通过 golangci-lint v2 检查，遵循 Go 最佳实践
- **高性能**：基准测试验证，内存分配优化，并发安全

---

## 包概览

包已按领域分组，便于查找和维护：

### Context（上下文与身份管理）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/context/xctx` | Context 增强（追踪/租户/平台） | Stable |
| `pkg/context/xtenant` | 租户信息中间件 | Stable |
| `pkg/context/xplatform` | 平台信息管理 | Stable |
| `pkg/context/xenv` | 环境变量管理 | Stable |

### Observability（可观测性）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/observability/xlog` | 结构化日志 | Stable |
| `pkg/observability/xtrace` | 链路追踪中间件 | Stable |
| `pkg/observability/xmetrics` | 统一可观测性接口 | Stable |
| `pkg/observability/xsampling` | 采样策略 | Alpha |
| `pkg/observability/xrotate` | 日志轮转 | Stable |

### Resilience（弹性与容错）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/resilience/xbreaker` | 熔断器 | Beta |
| `pkg/resilience/xretry` | 重试策略 | Beta |
| `pkg/resilience/xlimit` | 分布式限流器（Token Bucket） | Beta |

### Storage（数据存储）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/storage/xcache` | 缓存抽象层（Redis/Memory） | Stable |
| `pkg/storage/xetcd` | etcd 客户端封装（含 Informer list+watch 缓存） | Beta |
| `pkg/storage/xmongo` | MongoDB 客户端封装 | Beta |
| `pkg/storage/xclickhouse` | ClickHouse 客户端封装 | Beta |

### Distributed（分布式协调）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/distributed/xdlock` | 分布式锁 | Beta |
| `pkg/distributed/xcron` | 分布式定时任务 | Beta |
| `pkg/distributed/xelection` | 基于 etcd 的分布式选主 | Beta |
| `pkg/distributed/xsemaphore` | Redis 分布式信号量（Lua + Fallback） | Beta |

### MQ（消息队列）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/mq/xkafka` | Kafka 客户端封装 | Beta |
| `pkg/mq/xpulsar` | Pulsar 客户端封装 | Beta |

### Config（配置管理）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/config/xconf` | 配置管理 | Beta |

### Business（业务公共能力）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/business/xauth` | 认证服务客户端（Token/平台信息/双层缓存） | Beta |

### Debug（调试）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/debug/xdbg` | 运行时调试服务（Unix Socket） | Beta |

### Lifecycle（进程生命周期）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/lifecycle/xrun` | 进程生命周期管理（errgroup + 信号处理） | Stable |
| `pkg/lifecycle/xhealth` | Kubernetes 健康探针（liveness/readiness/startup） | Beta |

### Security（安全）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/security/xtls` | TLS 配置与证书加载工具 | Beta |

### Util（通用工具）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/util/xfile` | 文件操作工具（路径安全） | Stable |
| `pkg/util/xid` | Sonyflake v2 分布式 ID 生成 | Beta |
| `pkg/util/xjson` | JSON 格式化工具 | Stable |
| `pkg/util/xkeylock` | 基于 key 的进程内互斥锁 | Beta |
| `pkg/util/xlru` | LRU 缓存（泛型 + TTL） | Stable |
| `pkg/util/xmac` | MAC 地址工具库（多格式解析、验证、序列化） | Beta |
| `pkg/util/xnet` | IP 地址工具库（net/netip） | Beta |
| `pkg/util/xpool` | 泛型 Worker Pool | Stable |
| `pkg/util/xproc` | 进程信息查询 | Stable |
| `pkg/util/xsys` | 系统资源限制管理 | Stable |
| `pkg/util/xutil` | 泛型工具函数 | Stable |

### Testkit（测试辅助）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/testkit/xetcdtest` | etcd 嵌入式测试桩（集成测试用） | Internal |
| `pkg/testkit/xredismock` | Redis Mock 客户端 | Internal |
| `pkg/distributed/xsemaphore/xsemaphoremock` | xsemaphore gomock 桩 | Internal |

完整公开 API 与稳定性列表见 [`docs/03-conventions/01-api.md`](docs/03-conventions/01-api.md)。

---

## 技术规范

### Go 版本

- **固定版本**：Go 1.25.10（流水线要求）
- **验证方法**：`go version` 确认运行时版本
- **1.23 兼容分支**：[`develop-1.23-release`](https://github.com/omeyang/XKit/tree/develop-1.23-release) 为 Go 1.23 用户提供功能等价版本（仅依赖版本上限不同）

### 代码质量

**使用 go-task 运行检查**：
```bash
# 完整检查
task check

# 提交前快速检查
task pre-commit

# CI 流水线检查
task ci
```

**单独任务**：
```bash
task lint          # Lint 检查
task test          # 运行测试
task test-cover    # 测试覆盖率
task test-race     # 数据竞争检测
task bench         # 性能基准测试
```

---

## 开发工作流

本项目遵循规范驱动开发工作流：

```
项目原则 → 搜索已有方案 → 需求规格 → 技术计划 → 任务拆解 → 执行实现
```

---

## 项目原则

本项目遵循以下核心原则：

1. **模块化设计**：独立、可复用，职责清晰
2. **API 稳定性优先**：语义化版本控制
3. **测试优先开发**：TDD 红-绿-重构循环，核心业务 ≥ 95%，整体 ≥ 90%
4. **代码质量标准**：golangci-lint v2，Go 最佳实践
5. **文档金标准**：12 个核心原则（单一职责、图表优先等）
6. **性能与资源效率**：基准测试，内存优化，并发安全
7. **依赖管理**：最小化外部依赖，版本锁定

---

## 贡献

贡献流程、测试规范、Code Review 要点见 [`docs/03-conventions/02-contributing.md`](docs/03-conventions/02-contributing.md)。

---

## 文档

文档分类入口：[`docs/00-index.md`](docs/00-index.md)

| 类别 | 位置 |
|---|---|
| 关键决策（ADR） | [`docs/01-decisions/`](docs/01-decisions/00-index.md) |
| 进度追踪 | [`docs/02-progress.md`](docs/02-progress.md) |
| 约定规范（API / 贡献） | [`docs/03-conventions/`](docs/03-conventions/) |
| 跨包概念 | [`docs/05-concepts/`](docs/05-concepts/00-index.md) |
| 设计模式 | [`docs/06-patterns/`](docs/06-patterns/00-index.md) |
| 包详情 | [`docs/04-packages/`](docs/04-packages/00-index.md) |
