# XKit 公开 API 清单

## 设计原则

XKit 遵循 Go 语言的可见性规则：

- **公开 API**：大写开头的类型、函数、方法，可被外部包导入使用
- **私有实现**：小写开头的类型、函数、方法，仅包内可见

---

## 包清单

### Context（上下文与身份管理）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/context/xctx` | Context 增强（追踪/租户/平台） | Stable |
| `pkg/context/xtenant` | 租户信息中间件 | Stable |
| `pkg/context/xplatform` | 平台信息管理 | Stable |
| `pkg/context/xenv` | 环境变量管理 | Stable |

### Observability（可观测性）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/observability/xlog` | 结构化日志 | Stable |
| `pkg/observability/xtrace` | 链路追踪中间件 | Stable |
| `pkg/observability/xmetrics` | 统一可观测性接口 | Stable |
| `pkg/observability/xsampling` | 采样策略 | Alpha |
| `pkg/observability/xrotate` | 日志轮转 | Stable |

### Resilience（弹性与容错）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/resilience/xbreaker` | 熔断器 | Beta |
| `pkg/resilience/xretry` | 重试策略 | Beta |

### Storage（数据存储）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/storage/xcache` | 缓存抽象层（Redis/Memory） | Stable |
| `pkg/storage/xetcd` | etcd 客户端封装 | Beta |
| `pkg/storage/xmongo` | MongoDB 客户端封装 | Beta |
| `pkg/storage/xclickhouse` | ClickHouse 客户端封装 | Beta |

### Distributed（分布式协调）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/distributed/xdlock` | 分布式锁 | Beta |
| `pkg/distributed/xcron` | 分布式定时任务 | Beta |

### MQ（消息队列）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/mq/xkafka` | Kafka 客户端封装 | Beta |
| `pkg/mq/xpulsar` | Pulsar 客户端封装 | Beta |

### Config（配置管理）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/config/xconf` | 配置管理 | Beta |

### Util（通用工具）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/util/xfile` | 文件操作工具 | Stable |

---

## 核心接口概览

### pkg/storage/xcache

**接口**：
- `Loader` - Cache-Aside 模式加载器
- `Redis` - Redis 缓存接口
- `Memory` - 内存缓存接口

**工厂函数**：
- `NewLoader(cache Redis, opts ...LoaderOption) Loader`
- `NewRedis(client redis.UniversalClient, opts ...RedisOption) (Redis, error)`
- `NewMemory(opts ...MemoryOption) (Memory, error)`

### pkg/storage/xetcd

**类型**：
- `Client` - etcd 客户端封装
- `Config` - etcd 配置
- `Event` - Watch 事件

**工厂函数**：
- `NewClient(config *Config, opts ...Option) (*Client, error)`
- `DefaultConfig() *Config`

### pkg/observability/xlog

**接口**：
- `Logger` - 日志记录器接口
- `LoggerWithLevel` - 带级别控制的日志记录器
- `Leveler` - 级别控制接口

**工厂函数**：
- `New() *Builder`
- `Default() LoggerWithLevel`
- `SetDefault(l LoggerWithLevel)`

### pkg/context/xctx

**核心函数**：
- `WithTraceID/WithSpanID/WithRequestID` - 注入追踪信息
- `TraceID/SpanID/RequestID` - 提取追踪信息
- `EnsureTrace` - 确保追踪信息存在
- `WithTenantID/WithTenantName` - 注入租户信息
- `TenantID/TenantName` - 提取租户信息
- `WithPlatformID/WithDeploymentType` - 注入平台信息
- `PlatformID/DeploymentType` - 提取平台信息

### pkg/observability/xtrace

**中间件**：
- `HTTPMiddleware()` - HTTP 中间件
- `HTTPMiddlewareWithOptions(opts ...MiddlewareOption)` - 带配置的 HTTP 中间件
- `GRPCUnaryServerInterceptor()` - gRPC 一元服务端拦截器
- `GRPCStreamServerInterceptor()` - gRPC 流式服务端拦截器
- `GRPCUnaryClientInterceptor()` - gRPC 一元客户端拦截器
- `GRPCStreamClientInterceptor()` - gRPC 流式客户端拦截器

### pkg/context/xtenant

**中间件**：
- `HTTPMiddleware()` - HTTP 租户中间件
- `HTTPMiddlewareWithOptions(opts ...MiddlewareOption)` - 带配置的 HTTP 中间件
- `GRPCUnaryServerInterceptor()` - gRPC 一元服务端拦截器
- `GRPCStreamServerInterceptor()` - gRPC 流式服务端拦截器
- `GRPCUnaryClientInterceptor()` - gRPC 一元客户端拦截器
- `GRPCStreamClientInterceptor()` - gRPC 流式客户端拦截器

### pkg/context/xenv

**函数**：
- `Init() error / MustInit()` - 初始化
- `Type() DeployType / RequireType() (DeployType, error)` - 获取部署类型
- `IsLocal() bool / IsSaaS() bool` - 判断部署类型

### pkg/context/xplatform

**函数**：
- `Init(cfg Config) error / MustInit(cfg Config)` - 初始化
- `PlatformID() string / RequirePlatformID() (string, error)` - 获取平台 ID
- `HasParent() bool` - 判断是否有父平台
- `UnclassRegionID() string` - 获取非密区域 ID

### pkg/resilience/xretry

**接口**：
- `RetryPolicy` - 重试策略接口
- `BackoffPolicy` - 退避策略接口

**工厂函数**：
- `NewRetryer(opts ...RetryerOption) *Retryer`
- `NewFixedRetry(maxAttempts int) *FixedRetry`
- `NewAlwaysRetry() *AlwaysRetry`
- `NewNeverRetry() *NeverRetry`
- `NewFixedBackoff(delay time.Duration) *FixedBackoff`
- `NewExponentialBackoff(opts ...ExponentialBackoffOption) *ExponentialBackoff`
- `NewLinearBackoff(initial, increment, max time.Duration) *LinearBackoff`

### pkg/resilience/xbreaker

**接口**：
- `TripPolicy` - 熔断触发策略
- `SuccessPolicy` - 成功判定策略

**工厂函数**：
- `NewBreaker(name string, opts ...BreakerOption) *Breaker`
- `NewConsecutiveFailures(threshold uint32) *ConsecutiveFailures`
- `NewFailureCount(threshold uint32) *FailureCount`
- `NewFailureRatio(ratio float64, minRequests uint32) *FailureRatio`

### pkg/mq/xkafka

**接口**：
- `Producer` - 生产者接口
- `Consumer` - 消费者接口
- `ConsumerWithDLQ` - 支持死信队列的消费者

**工厂函数**：
- `NewProducer(config *kafka.ConfigMap, opts ...ProducerOption) (Producer, error)`
- `NewConsumer(config *kafka.ConfigMap, topics []string, opts ...ConsumerOption) (Consumer, error)`
- `NewTracingProducer(config *kafka.ConfigMap, opts ...ProducerOption) (*TracingProducer, error)`
- `NewTracingConsumer(config *kafka.ConfigMap, topics []string, opts ...ConsumerOption) (*TracingConsumer, error)`
- `NewConsumerWithDLQ(config *kafka.ConfigMap, topics []string, dlqPolicy *DLQPolicy, opts ...ConsumerOption) (ConsumerWithDLQ, error)`

### pkg/mq/xpulsar

**接口**：
- `Client` - Pulsar 客户端接口

**工厂函数**：
- `NewClient(opts ...ClientOption) (Client, error)`
- `WrapProducer(producer pulsar.Producer, topic string, tracer Tracer, observer xmetrics.Observer) *TracingProducer`
- `WrapConsumer(consumer pulsar.Consumer, topic string, tracer Tracer, observer xmetrics.Observer) *TracingConsumer`
- `NewTracingProducer(client Client, options pulsar.ProducerOptions, tracer Tracer, observer xmetrics.Observer) (*TracingProducer, error)`
- `NewTracingConsumer(client Client, options pulsar.ConsumerOptions, tracer Tracer, observer xmetrics.Observer) (*TracingConsumer, error)`

---

## 兼容性承诺

- **Stable** 包：遵循语义版本控制，不做破坏性变更
- **Beta** 包：API 可能调整，会提前在 CHANGELOG 中说明
- **Alpha** 包：实验性，API 可能随时变化