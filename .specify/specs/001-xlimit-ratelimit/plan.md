# xlimit 技术实现计划

## 元数据

| 字段 | 值 |
|------|-----|
| **Feature ID** | `feature-001-xlimit-ratelimit` |
| **关联 Spec** | [spec.md](./spec.md) |
| **创建日期** | 2026-01-16 |
| **状态** | Draft |

---

## 1. 技术上下文

### 1.1 项目环境

| 项目 | 值 |
|------|-----|
| Go 版本 | 1.23.12（流水线要求） |
| 目标路径 | `pkg/resilience/xlimit/` |
| 依赖模块 | xtenant, xctx, xlog, xconf |
| 底层库 | go-redis/redis_rate v10 |

### 1.2 现有依赖分析

```bash
# XKit 已有的相关依赖
github.com/redis/go-redis/v9          # Redis 客户端（已有）
github.com/go-redis/redis_rate/v10    # 限流库（需新增，同组织）
go.opentelemetry.io/otel              # 追踪（已有）
github.com/prometheus/client_golang   # 指标（已有）
```

### 1.3 相关模块接口

```go
// xtenant - 租户信息提取
xtenant.TenantID(ctx context.Context) string
xtenant.TenantName(ctx context.Context) string

// xctx - 调用方信息
xctx.CallerID(ctx context.Context) string

// xlog - 结构化日志
xlog.Warn(ctx, "message", "key", value)
```

---

## 2. Constitution 检查

### 2.1 原则符合性

| 原则 | 要求 | 符合性 | 说明 |
|------|------|--------|------|
| I. 模块化设计 | 单一职责、独立测试 | ✅ | xlimit 专注限流，与 xbreaker/xretry 独立 |
| II. API 稳定性 | 语义化版本 | ✅ | 遵循 Options 模式，便于扩展 |
| III. 测试优先 | TDD、覆盖率 ≥95% | ✅ | 先写测试，表驱动测试 |
| IV. 代码质量 | golangci-lint 通过 | ✅ | 中文注释，doc.go |
| V. 文档金标准 | 示例完整 | ✅ | example_test.go |
| VI. 性能效率 | 基准测试 | ✅ | 热路径优化，sync.Pool |
| VII. 依赖管理 | 最小依赖 | ✅ | 仅新增 redis_rate（同组织） |

### 2.2 质量门禁

| 检查项 | 目标 |
|--------|------|
| 测试覆盖率（核心） | ≥ 95% |
| 测试覆盖率（整体） | ≥ 90% |
| golangci-lint | 零错误、零警告 |
| go test -race | 无数据竞争 |
| 基准测试 | P99 < 5ms |

---

## 3. 研究阶段（Phase 0）

### 3.1 go-redis/redis_rate 分析

**仓库信息**：
- GitHub: https://github.com/go-redis/redis_rate
- Stars: 985+
- 最新版本: v10（2024）
- 维护状态: 活跃

**核心 API**：

```go
import "github.com/go-redis/redis_rate/v10"

// 创建限流器
limiter := redis_rate.NewLimiter(rdb)

// 检查限流
res, err := limiter.Allow(ctx, key, redis_rate.PerSecond(100))

// 结果结构
type Result struct {
    Allowed     int           // 本次允许的请求数
    Remaining   int           // 剩余配额
    RetryAfter  time.Duration // 重试等待时间
    ResetAfter  time.Duration // 配额重置时间
}

// 速率定义
redis_rate.PerSecond(100)  // 100 req/s
redis_rate.PerMinute(1000) // 1000 req/min
redis_rate.Limit{
    Rate:   100,
    Burst:  150,
    Period: time.Second,
}
```

**算法**：GCRA（Generic Cell Rate Algorithm）
- 漏桶算法变体
- 支持突发容量（Burst）
- 原子操作（Lua 脚本）

### 3.2 限流键设计

**模板变量**：

| 变量 | 来源 | 示例 |
|------|------|------|
| `${tenant_id}` | xtenant.TenantID(ctx) | `abc123` |
| `${caller_id}` | xctx.CallerID(ctx) | `order-service` |
| `${method}` | HTTP/gRPC Method | `POST` |
| `${path}` | HTTP/gRPC Path | `/v1/users` |
| `${resource}` | 业务自定义 | `createOrder` |

**模板解析**：使用 strings.ReplaceAll 替换变量

```go
// 输入模板
"tenant:${tenant_id}:api:${method}:${path}"

// 输出键
"tenant:abc123:api:POST:/v1/users"

// Redis 完整键（带前缀）
"ratelimit:tenant:abc123:api:POST:/v1/users"
```

### 3.3 配置覆盖匹配

**匹配优先级**（从高到低）：

1. 精确匹配：`tenant:vip-corp:api:POST:/v1/orders`
2. 部分通配：`tenant:vip-corp:api:POST:*`
3. 前缀通配：`tenant:vip-corp:*`
4. 全通配：`tenant:*:api:POST:/v1/orders`
5. 默认值：规则的 Limit 字段

**通配符规则**：
- `*` 匹配任意字符串（单段）
- 从左到右逐段匹配
- 第一个匹配的 Override 生效

### 3.4 降级策略实现

```go
type fallbackLimiter struct {
    local      Limiter        // 本地限流器
    distributed Limiter       // 分布式限流器
    strategy   FallbackStrategy
    podCount   int
}

func (f *fallbackLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
    result, err := f.distributed.Allow(ctx, key)
    if err == nil {
        return result, nil
    }

    // Redis 不可用，执行降级
    switch f.strategy {
    case FallbackLocal:
        // 本地配额 = 分布式配额 / Pod 数量
        return f.local.Allow(ctx, key)
    case FallbackOpen:
        return &Result{Allowed: true}, nil
    case FallbackClose:
        return &Result{Allowed: false}, ErrRedisUnavailable
    }
}
```

---

## 4. 设计阶段（Phase 1）

### 4.1 模块架构

```
pkg/resilience/xlimit/
├── doc.go                    # 包文档
├── limiter.go                # Limiter 接口定义
├── distributed.go            # 分布式限流器实现
├── local.go                  # 本地限流器实现
├── fallback.go               # 降级包装器
├── rule.go                   # Rule 结构和构建器
├── rule_matcher.go           # 规则匹配器
├── key.go                    # Key 结构和渲染
├── key_extractor.go          # HTTP/gRPC 键提取器
├── result.go                 # Result 结构
├── config.go                 # Config 结构
├── config_loader.go          # 配置加载器接口
├── config_loader_file.go     # 文件配置加载器
├── config_loader_etcd.go     # etcd 配置加载器
├── options.go                # Option 函数
├── errors.go                 # 错误定义
├── metrics.go                # Prometheus 指标
├── middleware_http.go        # HTTP 中间件
├── middleware_grpc.go        # gRPC 拦截器
├── middleware_options.go     # 中间件 Option
├── example_test.go           # 使用示例
├── limiter_test.go           # 限流器测试
├── rule_test.go              # 规则测试
├── key_test.go               # 键测试
├── middleware_http_test.go   # HTTP 中间件测试
├── middleware_grpc_test.go   # gRPC 拦截器测试
├── benchmark_test.go         # 基准测试
└── internal/
    └── wildcard/
        └── matcher.go        # 通配符匹配器
```

### 4.2 核心类型定义

```go
// ==================== limiter.go ====================

// Limiter 限流器接口
type Limiter interface {
    // Allow 检查是否允许请求通过
    Allow(ctx context.Context, key Key) (*Result, error)

    // AllowN 批量请求 n 个配额
    AllowN(ctx context.Context, key Key, n int) (*Result, error)

    // Reset 重置指定 key 的限流计数
    Reset(ctx context.Context, key Key) error

    // Close 关闭限流器
    Close() error
}

// ==================== rule.go ====================

// Rule 限流规则
type Rule struct {
    Name        string        `json:"name" yaml:"name"`
    KeyTemplate string        `json:"key_template" yaml:"key_template"`
    Limit       int           `json:"limit" yaml:"limit"`
    Window      time.Duration `json:"window" yaml:"window"`
    Burst       int           `json:"burst,omitempty" yaml:"burst,omitempty"`
    Overrides   []Override    `json:"overrides,omitempty" yaml:"overrides,omitempty"`
    Enabled     *bool         `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// Override 覆盖配置
type Override struct {
    Match  string        `json:"match" yaml:"match"`
    Limit  int           `json:"limit" yaml:"limit"`
    Window time.Duration `json:"window,omitempty" yaml:"window,omitempty"`
    Burst  int           `json:"burst,omitempty" yaml:"burst,omitempty"`
}

// ==================== key.go ====================

// Key 限流键
type Key struct {
    Tenant   string
    Caller   string
    Method   string
    Path     string
    Resource string
    Extra    map[string]string
}

// ==================== result.go ====================

// Result 限流结果
type Result struct {
    Allowed    bool
    Limit      int
    Remaining  int
    ResetAt    time.Time
    RetryAfter time.Duration
    Rule       string
    Key        string
}

// ==================== config.go ====================

// Config 限流器配置
type Config struct {
    KeyPrefix     string           `json:"key_prefix" yaml:"key_prefix"`
    Rules         []Rule           `json:"rules" yaml:"rules"`
    Fallback      FallbackStrategy `json:"fallback" yaml:"fallback"`
    LocalPodCount int              `json:"local_pod_count" yaml:"local_pod_count"`
    EnableMetrics bool             `json:"enable_metrics" yaml:"enable_metrics"`
    EnableHeaders bool             `json:"enable_headers" yaml:"enable_headers"`
}

// FallbackStrategy 降级策略
type FallbackStrategy string

const (
    FallbackLocal FallbackStrategy = "local"
    FallbackOpen  FallbackStrategy = "fail-open"
    FallbackClose FallbackStrategy = "fail-close"
)
```

### 4.3 工厂方法设计

```go
// New 创建分布式限流器
func New(rdb redis.UniversalClient, opts ...Option) (Limiter, error) {
    cfg := defaultOptions()
    for _, opt := range opts {
        opt(cfg)
    }

    // 创建底层 redis_rate 限流器
    rateLimiter := redis_rate.NewLimiter(rdb)

    // 创建规则匹配器
    matcher := newRuleMatcher(cfg.rules)

    // 创建分布式限流器
    distributed := &distributedLimiter{
        limiter:   rateLimiter,
        matcher:   matcher,
        keyPrefix: cfg.keyPrefix,
    }

    // 如果配置了降级策略，包装降级逻辑
    if cfg.fallback != "" {
        local := newLocalLimiter(cfg.rules, cfg.localPodCount)
        return &fallbackLimiter{
            distributed: distributed,
            local:       local,
            strategy:    cfg.fallback,
        }, nil
    }

    return distributed, nil
}

// NewLocal 创建本地限流器
func NewLocal(opts ...Option) (Limiter, error)
```

### 4.4 层级限流实现

```go
// hierarchicalLimiter 层级限流器
type hierarchicalLimiter struct {
    limiters []namedLimiter
}

type namedLimiter struct {
    name    string
    limiter Limiter
}

func (h *hierarchicalLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
    // 按顺序检查每一层，任一层拒绝则返回
    for _, nl := range h.limiters {
        result, err := nl.limiter.Allow(ctx, key)
        if err != nil {
            return nil, err
        }
        if !result.Allowed {
            result.Rule = nl.name
            return result, nil
        }
    }

    // 所有层都通过
    return &Result{Allowed: true}, nil
}
```

### 4.5 HTTP 中间件实现

```go
func HTTPMiddleware(limiter Limiter, opts ...MiddlewareOption) func(http.Handler) http.Handler {
    cfg := defaultMiddlewareOptions()
    for _, opt := range opts {
        opt(cfg)
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()

            // 检查是否跳过
            if cfg.skipper != nil && cfg.skipper(r) {
                next.ServeHTTP(w, r)
                return
            }

            // 提取限流键
            key := cfg.keyExtractor.Extract(ctx)
            if cfg.keyFunc != nil {
                key = cfg.keyFunc(r)
            }

            // 检查限流
            result, err := limiter.Allow(ctx, key)
            if err != nil {
                cfg.errorHandler(w, r, err)
                return
            }

            // 添加响应头
            if cfg.enableHeaders {
                result.SetHeaders(w)
            }

            if !result.Allowed {
                cfg.errorHandler(w, r, NewRateLimitError(result))
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

### 4.6 Prometheus 指标

```go
var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "xlimit",
            Name:      "requests_total",
            Help:      "Total number of rate limit checks",
        },
        []string{"rule", "allowed"},
    )

    remainingGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: "xlimit",
            Name:      "remaining_ratio",
            Help:      "Ratio of remaining quota",
        },
        []string{"rule"},
    )

    latencyHistogram = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "xlimit",
            Name:      "check_duration_seconds",
            Help:      "Duration of rate limit check",
            Buckets:   []float64{.0001, .0005, .001, .005, .01},
        },
        []string{"rule"},
    )
)
```

---

## 5. 依赖分析

### 5.1 新增依赖

| 依赖 | 版本 | 用途 | 理由 |
|------|------|------|------|
| github.com/go-redis/redis_rate/v10 | v10.x | GCRA 限流算法 | go-redis 官方维护，与现有 go-redis 同组织 |

### 5.2 现有依赖复用

| 依赖 | 用途 |
|------|------|
| github.com/redis/go-redis/v9 | Redis 客户端 |
| go.opentelemetry.io/otel | 追踪集成 |
| github.com/prometheus/client_golang | 指标导出 |
| gopkg.in/yaml.v3 | 配置解析 |
| go.etcd.io/etcd/client/v3 | etcd 配置加载 |

### 5.3 内部依赖

```
xlimit
├── xtenant (租户信息提取)
├── xctx (调用方信息提取)
├── xlog (结构化日志)
└── xconf (配置加载，可选)
```

---

## 6. 实现计划

### 6.1 文件实现顺序

```
Phase 1: 核心类型（无外部依赖）
├── errors.go           # 错误定义
├── key.go              # Key 结构
├── result.go           # Result 结构
├── rule.go             # Rule 结构和构建器
└── config.go           # Config 结构

Phase 2: 核心逻辑
├── internal/wildcard/  # 通配符匹配器
├── rule_matcher.go     # 规则匹配器
├── local.go            # 本地限流器
├── distributed.go      # 分布式限流器
└── fallback.go         # 降级包装器

Phase 3: 工厂和配置
├── options.go          # Option 函数
├── limiter.go          # 工厂方法
├── config_loader.go    # 配置加载器接口
├── config_loader_file.go
└── config_loader_etcd.go

Phase 4: 中间件
├── key_extractor.go    # 键提取器
├── middleware_options.go
├── middleware_http.go
└── middleware_grpc.go

Phase 5: 可观测性
├── metrics.go          # Prometheus 指标
└── doc.go              # 包文档

Phase 6: 测试和文档
├── *_test.go           # 单元测试
├── benchmark_test.go   # 基准测试
└── example_test.go     # 示例测试
```

### 6.2 测试策略

| 测试类型 | 文件 | 覆盖内容 |
|---------|------|---------|
| 单元测试 | key_test.go | 键渲染、变量替换 |
| 单元测试 | rule_test.go | 规则构建、验证 |
| 单元测试 | rule_matcher_test.go | 匹配逻辑、覆盖优先级 |
| 单元测试 | local_test.go | 本地限流算法 |
| 单元测试 | distributed_test.go | 分布式限流（Mock Redis） |
| 单元测试 | fallback_test.go | 降级切换逻辑 |
| 集成测试 | integration_test.go | 完整流程（需 Redis） |
| 中间件测试 | middleware_http_test.go | HTTP 拦截逻辑 |
| 中间件测试 | middleware_grpc_test.go | gRPC 拦截逻辑 |
| 基准测试 | benchmark_test.go | 性能验证 |
| 示例测试 | example_test.go | API 使用示例 |

### 6.3 Mock 策略

| 依赖 | Mock 方式 |
|------|----------|
| Redis | miniredis 或 go.uber.org/mock |
| xtenant | 直接注入 Context |
| xctx | 直接注入 Context |
| time | 自定义 Clock 接口 |

---

## 7. ADR（架构决策记录）

### ADR-001: 选择 go-redis/redis_rate 作为底层实现

**状态**：已批准

**上下文**：
需要选择分布式限流的底层实现。候选方案包括：
1. go-redis/redis_rate
2. gubernator-io/gubernator
3. 自建 Redis + Lua

**决策**：
选择 go-redis/redis_rate

**理由**：
- go-redis 官方维护（985 stars），与现有 go-redis 同组织
- XKit 已依赖 go-redis，零新增供应商
- GCRA 算法成熟，支持 Burst
- API 简洁，易于封装

**后果**：
- 正面：降低维护风险，简化集成
- 负面：受限于 GCRA 算法，无法使用其他算法

---

### ADR-002: 多租户配置采用默认值 + 覆盖模式

**状态**：已批准

**上下文**：
需要设计多租户限流配置方案。要求：
- 大部分租户使用默认配额
- 特殊租户可单独配置

**决策**：
每个 Rule 有默认 Limit，通过 Overrides 列表支持覆盖

**配置示例**：
```yaml
- name: tenant
  key_template: "tenant:${tenant_id}"
  limit: 1000          # 默认
  overrides:
    - match: "tenant:vip-corp"
      limit: 5000      # VIP 覆盖
```

**理由**：
- 简化配置：大部分租户无需单独配置
- 灵活覆盖：支持精确匹配和通配符
- 运行时高效：预编译匹配器

**后果**：
- 正面：配置简洁，易于理解
- 负面：通配符匹配有性能开销（可优化）

---

### ADR-003: 降级策略默认使用 local

**状态**：已批准

**上下文**：
Redis 不可用时需要降级方案。选项：
1. fail-open：直接放行
2. fail-close：直接拒绝
3. local：回退到本地限流

**决策**：
默认使用 local 策略

**计算方式**：
```
本地配额 = 分布式配额 / 预期 Pod 数量
```

**理由**：
- fail-open 可能被攻击利用
- fail-close 影响可用性
- local 兼顾安全性和可用性

**后果**：
- 正面：平衡安全与可用
- 负面：需要配置预期 Pod 数量

---

### ADR-004: 层级限流采用串行检查

**状态**：已批准

**上下文**：
层级限流（全局 → 租户 → API）的检查方式

**决策**：
串行检查，任一层拒绝即返回

**实现**：
```go
for _, rule := range rules {
    result := check(rule)
    if !result.Allowed {
        return result  // 立即返回，不继续检查
    }
}
```

**理由**：
- 语义清晰：任一层超限即拒绝
- 性能优化：拒绝时提前返回
- 简化实现：无需聚合多层结果

**后果**：
- 正面：实现简单，性能好
- 负面：无法获取所有层的剩余配额（仅返回触发层）

---

## 8. 风险缓解

| 风险 | 缓解措施 | 验证方法 |
|------|---------|---------|
| Redis 单点故障 | local 降级策略 | 故障注入测试 |
| 配置错误 | 配置校验 + dry-run | 单元测试 |
| 性能瓶颈 | Lua 原子操作 | 基准测试 |
| 时钟漂移 | 使用 Redis TIME | 集成测试 |

---

## 9. 验收检查清单

### 9.1 功能验收

- [ ] 分布式限流正常工作
- [ ] 本地限流正常工作
- [ ] 层级限流正常工作
- [ ] 配置覆盖正常工作
- [ ] 降级切换正常工作
- [ ] HTTP 中间件正常工作
- [ ] gRPC 拦截器正常工作
- [ ] 动态配置热更新正常工作

### 9.2 质量验收

- [ ] 测试覆盖率（核心）≥ 95%
- [ ] 测试覆盖率（整体）≥ 90%
- [ ] golangci-lint 零错误
- [ ] go test -race 无竞争
- [ ] 基准测试 P99 < 5ms

### 9.3 文档验收

- [ ] doc.go 包文档完整
- [ ] example_test.go 示例完整
- [ ] 中文注释完整
- [ ] README 更新

---

## 10. 相关文档

| 文档 | 路径 |
|------|------|
| 需求规格 | [spec.md](./spec.md) |
| 项目宪法 | `.specify/memory/constitution.md` |
| 测试标准 | `.specify/prior-art/01-standards/03-testing/02-testing-standards.md` |
| 熔断器参考 | `pkg/resilience/xbreaker/` |
| 重试参考 | `pkg/resilience/xretry/` |
