# XKit

XKit 是独立的 Go 基础工具库，为业务开发提供通用功能支持。文档只关注当下可用能力与使用方式，不引入历史演变。

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
| `pkg/storage/xetcd` | etcd 客户端封装 | Beta |
| `pkg/storage/xmongo` | MongoDB 客户端封装 | Beta |
| `pkg/storage/xclickhouse` | ClickHouse 客户端封装 | Beta |

### Distributed（分布式协调）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/distributed/xdlock` | 分布式锁 | Beta |
| `pkg/distributed/xcron` | 分布式定时任务 | Beta |

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

### Util（通用工具）

| 包 | 用途 | 稳定性 |
| --- | --- | --- |
| `pkg/util/xfile` | 文件操作工具（路径安全） | Stable |
| `pkg/util/xjson` | JSON 格式化工具 | Stable |
| `pkg/util/xkeylock` | 基于 key 的进程内互斥锁 | Beta |
| `pkg/util/xlru` | LRU 缓存（泛型 + TTL） | Stable |
| `pkg/util/xmac` | MAC 地址工具库（多格式解析、验证、序列化） | Beta |
| `pkg/util/xnet` | IP 地址工具库（net/netip） | Beta |
| `pkg/util/xpool` | 泛型 Worker Pool | Stable |
| `pkg/util/xproc` | 进程信息查询 | Stable |
| `pkg/util/xsys` | 系统资源限制管理 | Stable |
| `pkg/util/xutil` | 泛型工具函数 | Stable |

完整公开 API 与稳定性列表见 `docs/API.md`。

---

## 技术规范

### Go 版本

- **固定版本**：Go 1.25.0（流水线要求）
- **验证方法**：`go version` 确认运行时版本

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

## 贡献指南

### 提交前检查

**本地检查**（必须通过）：
```bash
task pre-commit  # 快速检查
task ci          # 完整检查
```

### Code Review 要求

- [ ] 代码符合项目原则（constitution.md）
- [ ] 测试覆盖率达标（核心业务 ≥ 95%，整体 ≥ 90%）
- [ ] golangci-lint 检查通过
- [ ] 文档完整（代码注释 + 技术文档）
- [ ] 无明显性能问题
- [ ] 错误处理完善
- [ ] 并发安全（如适用）

### Merge Request 流程

1. 创建分支：`git checkout -b feature/xxx`
2. 执行开发工作流
3. 提交代码（遵循 Conventional Commits 规范）
4. 创建 MR
5. Code Review 通过后合并

---

## 文档

- **API 文档**：`docs/API.md`
- **命名规范**：`docs/NAMING.md`
