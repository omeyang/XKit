# Feature: xlimit 分布式限流模块

## 元数据

| 字段 | 值 |
|------|-----|
| **Feature ID** | `feature-001-xlimit-ratelimit` |
| **创建日期** | 2026-01-16 |
| **状态** | Draft |
| **负责人** | TBD |
| **AI 模型** | Claude Opus 4.5 |
| **审核签字** | [待签字] |

---

## 1. 需求溯源

### 1.1 业务背景

XKit 作为企业级 Go 基础工具库，服务于多租户、分布式、Kubernetes 部署场景。现有 `pkg/resilience` 模块已包含熔断器（xbreaker）和重试策略（xretry），但缺少限流能力。

### 1.2 需求来源

| 来源类型 | 描述 |
|---------|------|
| 内部需求 | 业务服务需要多租户粒度的限流保护 |
| 架构需求 | 与现有 xbreaker、xretry 形成完整的弹性防护体系 |
| 运维需求 | 需要支持动态配置调整，无需重启服务 |

### 1.3 核心痛点

1. **多租户公平性**：大租户可能耗尽共享配额，导致小租户服务不可用
2. **分布式一致性**：K8s 多 Pod 部署需要跨实例共享限流状态
3. **多维度限流**：需要按租户、调用方、API 等多维度组合限流
4. **动态配置**：生产环境需要热更新限流配置，无需重启
5. **降级保护**：Redis 不可用时需要有兜底方案

---

## 2. 差异化与创新点

### 2.1 业界方案对比

| 对比项 | uber-go/ratelimit | go-redis/redis_rate | sentinel-golang | xlimit (本方案) |
|-------|-------------------|---------------------|-----------------|----------------|
| **分布式支持** | ❌ 仅单机 | ✅ Redis | ⚠️ 需 Token Server | ✅ Redis + 本地降级 |
| **多租户原生** | ❌ | ❌ 需封装 | ⚠️ 热点参数 | ✅ 原生支持 |
| **层级限流** | ❌ | ❌ | ⚠️ 需配置 | ✅ 多层叠加 |
| **动态配置** | ❌ | ❌ | ✅ | ✅ xconf/etcd 集成 |
| **XKit 集成** | ❌ | ❌ | ❌ | ✅ xtenant/xctx/xlog |
| **维护状态** | 活跃 | 活跃 | 较慢 | - |

### 2.2 独特价值

1. **多租户原生支持**：与 xtenant 深度集成，自动提取租户信息，支持默认配额 + 租户覆盖配置
2. **层级限流架构**：支持全局 → 租户 → API 多层配额叠加，任一层触发即拒绝
3. **智能降级策略**：Redis 不可用时自动降级到本地限流，兼顾可用性和安全性
4. **零额外依赖**：基于 go-redis/redis_rate，XKit 已有 go-redis 依赖，无新增第三方库
5. **统一 API 风格**：遵循 XKit Options 模式，与 xbreaker、xretry 保持一致

---

## 3. User Stories

### US-1: 多租户限流保护

**用户角色**：平台运维工程师

**业务需求**：作为运维人员，我需要为不同租户配置不同的限流配额，防止某个租户的异常流量影响其他租户

**验收条件**：
- [ ] 可配置默认限流配额，适用于所有租户
- [ ] 可为特定租户配置覆盖配额（如 VIP 租户配额更高）
- [ ] 租户限流相互独立，一个租户触发限流不影响其他租户
- [ ] 限流配置可通过配置中心动态下发，无需重启服务

### US-2: API 级别细粒度限流

**用户角色**：后端开发工程师

**业务需求**：作为开发人员，我需要为不同 API 配置不同的限流策略，写操作限制更严格

**验收条件**：
- [ ] 支持按 HTTP Method + Path 或自定义资源名限流
- [ ] 支持租户 + API 组合维度限流
- [ ] 可配置写操作（POST/PUT/DELETE）比读操作限制更严格
- [ ] 提供 HTTP 中间件和 gRPC 拦截器，开箱即用

### US-3: 上游调用方限流

**用户角色**：服务治理架构师

**业务需求**：作为架构师，我需要限制上游服务的调用频率，防止某个批处理任务打爆下游服务

**验收条件**：
- [ ] 支持按调用方服务 ID 限流
- [ ] 可从 HTTP Header 或 Context 自动提取调用方信息
- [ ] 可配置不同调用方的差异化配额

### US-4: 高可用降级保护

**用户角色**：SRE 工程师

**业务需求**：作为 SRE，我需要确保 Redis 故障时限流功能有降级方案，不影响业务可用性

**验收条件**：
- [ ] Redis 不可用时自动降级到本地限流
- [ ] 支持配置降级策略：本地限流 / 直接放行 / 直接拒绝
- [ ] 本地限流配额按 Pod 数量均分
- [ ] Redis 恢复后自动切换回分布式限流

### US-5: 可观测性集成

**用户角色**：监控运维工程师

**业务需求**：作为监控人员，我需要看到限流的运行状态和触发情况

**验收条件**：
- [ ] 提供 Prometheus 指标：请求总数、限流触发次数、剩余配额比例
- [ ] 支持 OpenTelemetry 追踪集成
- [ ] 限流触发时记录结构化日志（与 xlog 集成）
- [ ] HTTP 响应包含标准限流头（X-RateLimit-*）

---

## 4. 验收标准（SMART）

### 4.1 功能指标

| 指标 | 目标值 | 验证方法 |
|------|--------|---------|
| 多租户隔离 | 100% 配额隔离 | 并发测试验证租户间无干扰 |
| 配置覆盖 | 支持精确匹配 + 通配符 | 单元测试覆盖各种匹配场景 |
| 层级限流 | 支持 ≥3 层叠加 | 集成测试验证多层生效 |
| 降级切换 | Redis 故障 <1s 自动降级 | 故障注入测试 |
| 配置热更新 | <5s 生效 | etcd watch 测试 |

### 4.2 非功能指标

| 指标 | 目标值 | 验证方法 |
|------|--------|---------|
| 单次限流检查延迟 | P99 < 5ms | 基准测试 |
| Redis 操作延迟 | P99 < 2ms（同机房） | 基准测试 |
| 内存占用 | < 100MB/100万 Key | 内存分析 |
| 吞吐量 | > 50,000 req/s（单实例） | 压力测试 |
| 代码覆盖率 | 核心逻辑 ≥95%，整体 ≥90% | go test -cover |

---

## 5. 风险与假设（RAC 矩阵）

### 5.1 风险

| ID | 风险 | 影响 | 概率 | 缓解措施 |
|----|------|------|------|---------|
| R1 | Redis 单点故障导致限流失效 | 高 | 中 | 实现本地降级策略，支持 fail-open/fail-close |
| R2 | 高并发下 Redis 成为瓶颈 | 中 | 低 | 使用 Lua 脚本原子操作，减少网络往返 |
| R3 | 配置错误导致误限流 | 高 | 中 | 提供配置校验，支持 dry-run 模式 |
| R4 | 时钟漂移影响限流准确性 | 低 | 低 | 使用 Redis TIME 命令，不依赖本地时钟 |

### 5.2 假设

| ID | 假设 | 依赖 | 验证方法 |
|----|------|------|---------|
| A1 | 业务已部署 Redis 并可用 | 基础设施 | 启动时健康检查 |
| A2 | 租户信息已通过 xtenant 注入 Context | XKit 规范 | 中间件自动检查 |
| A3 | Pod 数量可通过配置或环境变量获取 | K8s 部署 | 用于本地降级配额计算 |
| A4 | go-redis/redis_rate GCRA 算法满足精度要求 | 依赖库 | 基准测试验证 |

### 5.3 约束

| ID | 约束 | 说明 |
|----|------|------|
| C1 | Go 1.25+ | 项目最低版本要求 |
| C2 | 无新增外部依赖 | 仅使用 go-redis/redis_rate（go-redis 已有） |
| C3 | 遵循 XKit API 风格 | Options 模式、策略接口、错误哨兵 |
| C4 | 中文注释 | 项目规范要求 |

---

## 6. 功能需求

### 6.1 核心限流功能

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-1 | 支持分布式限流（基于 Redis） | P0 | 多 Pod 共享配额，误差 <1% |
| FR-2 | 支持本地限流（单 Pod） | P0 | 无 Redis 时独立运行 |
| FR-3 | 支持秒级和分钟级时间窗口 | P0 | PerSecond、PerMinute 配置 |
| FR-4 | 支持突发容量（Burst） | P1 | 允许短时超过 Limit |

### 6.2 多维度限流

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-5 | 支持租户维度限流 | P0 | 自动从 xtenant 提取 |
| FR-6 | 支持调用方维度限流 | P1 | 从 Header/Context 提取 |
| FR-7 | 支持 API/资源维度限流 | P0 | 支持 Method+Path 或自定义 |
| FR-8 | 支持多维度组合限流 | P0 | 键模板变量替换 |

### 6.3 配置管理

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-9 | 支持默认配额配置 | P0 | 规则级别默认值 |
| FR-10 | 支持租户/规则覆盖配置 | P0 | Override 精确匹配 + 通配符 |
| FR-11 | 支持层级限流（多规则叠加） | P0 | 任一层触发即拒绝 |
| FR-12 | 支持动态配置热更新 | P0 | etcd/xconf watch |

### 6.4 降级与容错

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-13 | 支持本地降级（local） | P0 | 配额按 Pod 数均分 |
| FR-14 | 支持直接放行（fail-open） | P1 | Redis 故障时放行 |
| FR-15 | 支持直接拒绝（fail-close） | P1 | Redis 故障时拒绝 |
| FR-16 | 支持自动故障恢复 | P0 | Redis 恢复后自动切换 |

### 6.5 中间件与集成

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-17 | 提供 HTTP 中间件 | P0 | 兼容 net/http |
| FR-18 | 提供 gRPC 拦截器 | P0 | 一元 + 流式 |
| FR-19 | 支持限流响应头 | P1 | X-RateLimit-* 标准头 |
| FR-20 | 支持自定义错误处理 | P1 | ErrorHandler 回调 |

### 6.6 可观测性

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| FR-21 | 支持 Prometheus 指标 | P1 | requests_total, remaining |
| FR-22 | 支持 OpenTelemetry 追踪 | P2 | Span 记录限流决策 |
| FR-23 | 支持结构化日志 | P1 | 与 xlog 集成 |

---

## 7. 非功能需求

| ID | 需求 | 优先级 | 验收标准 |
|----|------|--------|---------|
| NFR-1 | 线程安全 | P0 | 并发访问无数据竞争 |
| NFR-2 | 优雅关闭 | P0 | Close() 释放资源 |
| NFR-3 | Context 超时支持 | P0 | 尊重 ctx.Done() |
| NFR-4 | 零内存分配热路径 | P1 | sync.Pool 复用 |
| NFR-5 | 完整文档和示例 | P0 | doc.go + example_test.go |

---

## 8. API 设计概览

### 8.1 模块结构

```
pkg/resilience/xlimit/
├── doc.go                 # 包文档
├── limiter.go             # 核心限流器接口
├── rule.go                # 限流规则定义
├── key.go                 # 限流键生成
├── config.go              # 配置结构
├── result.go              # 限流结果
├── errors.go              # 错误定义
├── options.go             # Option 配置
├── fallback.go            # 降级策略
├── middleware_http.go     # HTTP 中间件
├── middleware_grpc.go     # gRPC 拦截器
└── example_test.go        # 使用示例
```

### 8.2 核心接口

```go
// Limiter 限流器接口
type Limiter interface {
    Allow(ctx context.Context, key Key) (*Result, error)
    AllowN(ctx context.Context, key Key, n int) (*Result, error)
    Reset(ctx context.Context, key Key) error
    Close() error
}

// 工厂方法
func New(rdb redis.UniversalClient, opts ...Option) (Limiter, error)
func NewLocal(opts ...Option) (Limiter, error)
```

### 8.3 规则构建器

```go
// 预定义规则
xlimit.GlobalRule(10000, time.Second)
xlimit.TenantRule(1000, time.Second)
xlimit.TenantAPIRule(100, time.Second)
xlimit.CallerRule(500, time.Second)

// 链式构建
xlimit.NewRule("tenant").
    KeyTemplate("tenant:${tenant_id}").
    Limit(1000).
    Window(time.Second).
    Override("tenant:vip-corp", 5000).
    Build()
```

### 8.4 中间件使用

```go
// HTTP
handler := xlimit.HTTPMiddleware(limiter,
    xlimit.WithRuleNames("tenant", "tenant-api"),
    xlimit.WithEnableHeaders(true),
)(mux)

// gRPC
grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        xlimit.UnaryServerInterceptor(limiter),
    ),
)
```

---

## 9. 技术决策

### ADR-1: 选择 go-redis/redis_rate 作为底层实现

**状态**：已批准

**背景**：需要选择分布式限流的底层实现方案

**决策**：使用 go-redis/redis_rate

**理由**：
- go-redis 官方维护，985 stars，持续更新
- XKit 已有 go-redis 依赖，零新增依赖
- GCRA 算法成熟，性能优秀
- API 简洁，易于封装

**替代方案**：
- gubernator：star 少，无大厂背书，风险高
- sentinel-golang：更新慢，最新版 2022 年
- 自建 Redis + Lua：工作量大，需要自己处理边界情况

### ADR-2: 多租户配置采用默认值 + 覆盖模式

**状态**：已批准

**背景**：需要设计多租户配额配置方案

**决策**：每个规则有默认配额，通过 Override 列表支持租户级覆盖

**理由**：
- 简化配置：大部分租户使用默认值
- 灵活覆盖：特殊租户可单独配置
- 支持通配符：如 `tenant:*:api:POST:/v1/*`

### ADR-3: 降级策略选择 local 作为默认值

**状态**：已批准

**背景**：Redis 不可用时需要降级方案

**决策**：默认使用 local（本地限流）策略

**理由**：
- 兼顾可用性和安全性
- 本地配额 = 分布式配额 / Pod 数量
- 比 fail-open（可能被攻击）和 fail-close（影响可用性）更平衡

---

## 10. AI 建议 vs 人工确认

| 技术决策 | AI 建议 | 人工确认 | 最终决定 | 说明 |
|---------|--------|---------|---------|------|
| 底层限流库 | gubernator | ❌ 拒绝 | go-redis/redis_rate | star 少，无大厂背书 |
| 限流算法 | GCRA（漏桶变体） | ✅ 确认 | GCRA | redis_rate 内置 |
| 多租户策略 | 默认 + 覆盖 | ✅ 确认 | 默认 + 覆盖 | 符合需求 |
| 降级策略 | local | ✅ 确认 | local | 平衡可用性和安全性 |
| 配置来源 | 动态配置 | ✅ 确认 | xconf/etcd | 支持热更新 |
| API 风格 | Options 模式 | ✅ 确认 | Options | 与 XKit 一致 |

---

## 11. 安全与审核

### 11.1 安全考虑

| 风险 | 缓解措施 |
|------|---------|
| 配置注入攻击 | 键模板变量严格校验，禁止特殊字符 |
| Redis 连接泄露 | 使用连接池，优雅关闭 |
| 限流绕过 | 中间件强制检查，无跳过路径 |
| 敏感信息泄露 | 日志脱敏，不记录完整 Key |

### 11.2 审核清单

- [ ] 代码符合项目原则（constitution.md）
- [ ] 测试覆盖率达标（核心 ≥95%，整体 ≥90%）
- [ ] golangci-lint 检查通过
- [ ] 文档完整（doc.go + example_test.go）
- [ ] 无明显性能问题（基准测试通过）
- [ ] 错误处理完善
- [ ] 并发安全（race detector 通过）

### 11.3 审核签字

| 角色 | 姓名 | 日期 | 签字 |
|------|------|------|------|
| 技术负责人 | | | |
| 安全审核 | | | |
| 架构审核 | | | |

---

## 12. 里程碑

| 阶段 | 交付物 | 状态 |
|------|--------|------|
| Phase 1 | spec.md 需求规格 | ✅ 完成 |
| Phase 2 | plan.md 技术计划 | 待开始 |
| Phase 3 | tasks.md 任务拆解 | 待开始 |
| Phase 4 | 代码实现 + 测试 | 待开始 |
| Phase 5 | 文档和示例 | 待开始 |
| Phase 6 | Code Review + 合并 | 待开始 |

---

## 附录

### A. 限流键模板变量

| 变量 | 来源 | 示例值 |
|------|------|--------|
| `${tenant_id}` | xtenant.TenantID(ctx) | `abc123` |
| `${caller_id}` | xctx.CallerID(ctx) | `order-service` |
| `${method}` | HTTP Method / gRPC Method | `POST` |
| `${path}` | HTTP Path / gRPC Service.Method | `/v1/users` |
| `${resource}` | 业务自定义 | `createOrder` |

### B. 配置示例

```yaml
ratelimit:
  key_prefix: "ratelimit"
  fallback: local
  local_pod_count: 10

  rules:
    - name: global
      key_template: "global"
      limit: 10000
      window: 1s

    - name: tenant
      key_template: "tenant:${tenant_id}"
      limit: 1000
      window: 1s
      overrides:
        - match: "tenant:vip-corp"
          limit: 5000
        - match: "tenant:free-tier"
          limit: 100

    - name: tenant-api
      key_template: "tenant:${tenant_id}:api:${method}:${path}"
      limit: 100
      window: 1s
      overrides:
        - match: "tenant:*:api:POST:/v1/orders"
          limit: 50
```

### C. 相关文档

- 项目宪法：`.specify/memory/constitution.md`
- 熔断器模块：`pkg/resilience/xbreaker/`
- 重试模块：`pkg/resilience/xretry/`
- 租户模块：`pkg/context/xtenant/`
