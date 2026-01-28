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
| `pkg/resilience/xlimit` | 分布式限流器（Token Bucket） | Beta |

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

### Business（业务公共能力）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/business/xauth` | 认证服务客户端（Token/平台信息/双层缓存） | Beta |

### Debug（调试）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/debug/xdbg` | 运行时调试服务（Unix Socket） | Beta |

### Lifecycle（进程生命周期）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/lifecycle/xrun` | 进程生命周期管理（errgroup + 信号处理） | Stable |

### Util（通用工具）

| 包名 | 用途 | 稳定性 |
|------|------|--------|
| `pkg/util/xfile` | 文件操作工具（路径安全） | Stable |
| `pkg/util/xjson` | JSON 格式化工具 | Stable |
| `pkg/util/xkeylock` | 基于 key 的进程内互斥锁 | Beta |
| `pkg/util/xlru` | LRU 缓存（泛型 + TTL） | Stable |
| `pkg/util/xnet` | IP 地址工具库（net/netip） | Beta |
| `pkg/util/xpool` | 泛型 Worker Pool | Stable |
| `pkg/util/xproc` | 进程信息查询 | Stable |
| `pkg/util/xsys` | 系统资源限制管理 | Stable |
| `pkg/util/xutil` | 泛型工具函数 | Stable |

---

## 核心接口概览

### pkg/context/xctx

**核心函数**：
- `WithTraceID/WithSpanID/WithRequestID` - 注入追踪信息
- `TraceID/SpanID/RequestID` - 提取追踪信息
- `EnsureTrace` - 确保追踪信息存在
- `WithTenantID/WithTenantName` - 注入租户信息
- `TenantID/TenantName` - 提取租户信息
- `WithPlatformID/WithDeploymentType` - 注入平台信息
- `PlatformID/DeploymentType` - 提取平台信息

### pkg/context/xtenant

**中间件**：
- `HTTPMiddleware()` - HTTP 租户中间件
- `HTTPMiddlewareWithOptions(opts ...MiddlewareOption)` - 带配置的 HTTP 中间件
- `GRPCUnaryServerInterceptor()` - gRPC 一元服务端拦截器
- `GRPCStreamServerInterceptor()` - gRPC 流式服务端拦截器
- `GRPCUnaryClientInterceptor()` - gRPC 一元客户端拦截器
- `GRPCStreamClientInterceptor()` - gRPC 流式客户端拦截器

### pkg/context/xplatform

**函数**：
- `Init(cfg Config) error / MustInit(cfg Config)` - 初始化
- `PlatformID() string / RequirePlatformID() (string, error)` - 获取平台 ID
- `HasParent() bool` - 判断是否有父平台
- `UnclassRegionID() string` - 获取非密区域 ID

### pkg/context/xenv

**函数**：
- `Init() error / MustInit()` - 初始化
- `Type() DeployType / RequireType() (DeployType, error)` - 获取部署类型
- `IsLocal() bool / IsSaaS() bool` - 判断部署类型

### pkg/observability/xlog

**接口**：
- `Logger` - 日志记录器接口
- `LoggerWithLevel` - 带级别控制的日志记录器
- `Leveler` - 级别控制接口

**工厂函数**：
- `New() *Builder` - 创建 Builder
- `Default() LoggerWithLevel` - 获取全局 Logger（惰性初始化）
- `SetDefault(l LoggerWithLevel)` - 替换全局 Logger
- `ResetDefault()` - 重置为未初始化状态（仅测试用）

**全局便利函数**：
- `Debug(ctx, msg, ...slog.Attr)` - Debug 级别日志
- `Info(ctx, msg, ...slog.Attr)` - Info 级别日志
- `Warn(ctx, msg, ...slog.Attr)` - Warn 级别日志
- `Error(ctx, msg, ...slog.Attr)` - Error 级别日志
- `Stack(ctx, msg, err)` - 带堆栈的错误日志

**延迟求值**：
- `Lazy/LazyString/LazyInt/LazyError/LazyDuration/LazyGroup`

**便捷属性**：
- `Err/Duration/Component/Operation/Count/UserID/StatusCode/Method/Path`

### pkg/observability/xtrace

**中间件**：
- `HTTPMiddleware()` - HTTP 中间件
- `HTTPMiddlewareWithOptions(opts ...MiddlewareOption)` - 带配置的 HTTP 中间件
- `GRPCUnaryServerInterceptor()` - gRPC 一元服务端拦截器
- `GRPCStreamServerInterceptor()` - gRPC 流式服务端拦截器
- `GRPCUnaryClientInterceptor()` - gRPC 一元客户端拦截器
- `GRPCStreamClientInterceptor()` - gRPC 流式客户端拦截器

### pkg/observability/xmetrics

**接口**：
- `Observer` - 统一观测接口
  - `Start(ctx context.Context, opts SpanOptions) (context.Context, Span)`
- `Span` - 观测 Span
  - `End(result Result)`

**类型**：
- `Kind` - Span 类型（KindInternal/KindServer/KindClient/KindProducer/KindConsumer）
- `Status` - 结果状态（StatusOK/StatusError）
- `Attr` - 属性键值对
- `SpanOptions` - Span 配置（Component/Operation/Kind/Attrs）
- `Result` - Span 结果（Status/Err/Attrs）
- `NoopObserver` - 空实现

**属性工厂函数**：
- `String/Bool/Int/Int64/Uint64/Float64/Duration/Any`

**OpenTelemetry 实现**：
- `NewOTelObserver(opts ...Option) (Observer, error)`
- `WithInstrumentationName(name string) Option`
- `WithTracerProvider(provider trace.TracerProvider) Option`
- `WithMeterProvider(provider metric.MeterProvider) Option`

### pkg/observability/xsampling

**接口**：
- `Sampler` - 采样策略接口
  - `ShouldSample(ctx context.Context) bool`
- `ResettableSampler` - 可重置的采样器

**基础采样器**：
- `Always() Sampler` - 全量采样
- `Never() Sampler` - 不采样
- `NewRateSampler(rate float64) *RateSampler` - 固定比率采样
- `NewCountSampler(n int) *CountSampler` - 计数采样（每 N 次采样一次）
- `NewProbabilitySampler(probability float64) *ProbabilitySampler` - 概率采样

**组合采样器**：
- `All(samplers ...Sampler) *CompositeSampler` - AND 组合（全部通过）
- `Any(samplers ...Sampler) *CompositeSampler` - OR 组合（任一通过）
- `NewCompositeSampler(mode CompositeMode, samplers ...Sampler) *CompositeSampler`

**基于 Key 的一致性采样**：
- `NewKeyBasedSampler(rate float64, keyFunc KeyFunc) *KeyBasedSampler`
- `KeyFunc = func(ctx context.Context) string` - Key 提取函数

### pkg/observability/xrotate

**接口**：
- `Rotator` - 日志轮转接口（实现 io.Writer + io.Closer）
  - `Write(p []byte) (n int, err error)` - 写入日志，触发条件时自动轮转
  - `Close() error` - 关闭轮转器
  - `Rotate() error` - 手动触发轮转

**工厂函数**：
- `NewLumberjack(filename string, opts ...LumberjackOption) (Rotator, error)`

**选项函数**：
- `WithMaxSize(mb int)` - 单文件最大大小（默认 500MB）
- `WithMaxBackups(n int)` - 保留备份数（默认 7）
- `WithMaxAge(days int)` - 备份保留天数（默认 30）
- `WithCompress(compress bool)` - 是否压缩（默认 true）
- `WithLocalTime(local bool)` - 备份名是否用本地时间（默认 false）
- `WithFileMode(mode os.FileMode)` - 文件权限（默认 0600）

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

### pkg/resilience/xlimit

**接口**：
- `Limiter` - 限流器接口（Allow/AllowN/Close）
- `RuleProvider` - 限流规则匹配

**工厂函数**：
- `New(rdb redis.UniversalClient, opts ...Option) (Limiter, error)` - Redis 限流器
- `NewLocal(opts ...Option) (Limiter, error)` - 本地限流器
- `NewWithFallback(rdb redis.UniversalClient, opts ...Option) (Limiter, error)` - 带降级的限流器
- `NewRule(name, keyTemplate string, limit int, window time.Duration) Rule` - 创建限流规则

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

### pkg/storage/xmongo

**接口**：
- `Mongo` - MongoDB 封装接口
  - `Client() *mongo.Client` - 获取底层客户端
  - `Health(ctx context.Context) error` - 健康检查
  - `Stats() Stats` - 统计信息
  - `Close(ctx context.Context) error` - 关闭连接
  - `FindPage(ctx, coll, filter, opts PageOptions) (*PageResult, error)` - 分页查询
  - `BulkWrite(ctx, coll, docs, opts BulkOptions) (*BulkResult, error)` - 批量写入

**工厂函数**：
- `New(client *mongo.Client, opts ...Option) (Mongo, error)`

**选项函数**：
- `WithHealthTimeout(timeout time.Duration)` - 健康检查超时
- `WithSlowQueryThreshold(threshold time.Duration)` - 慢查询阈值
- `WithSlowQueryHook(hook SlowQueryHook)` - 慢查询回调
- `WithAsyncSlowQueryHook(hook AsyncSlowQueryHook)` - 异步慢查询回调
- `WithObserver(observer xmetrics.Observer)` - 可观测性

### pkg/storage/xclickhouse

**接口**：
- `ClickHouse` - ClickHouse 封装接口
  - `Conn() driver.Conn` - 获取底层连接
  - `Health(ctx context.Context) error` - 健康检查
  - `Stats() Stats` - 统计信息
  - `Close() error` - 关闭连接
  - `QueryPage(ctx, query string, opts PageOptions, args ...any) (*PageResult, error)` - 分页查询
  - `BatchInsert(ctx, table string, rows []any, opts BatchOptions) (*BatchResult, error)` - 批量插入

**工厂函数**：
- `New(conn driver.Conn, opts ...Option) (ClickHouse, error)`

**选项函数**：
- `WithHealthTimeout(timeout time.Duration)` - 健康检查超时
- `WithSlowQueryThreshold(threshold time.Duration)` - 慢查询阈值
- `WithSlowQueryHook(hook SlowQueryHook)` - 慢查询回调
- `WithAsyncSlowQueryHook(hook AsyncSlowQueryHook)` - 异步慢查询回调
- `WithObserver(observer xmetrics.Observer)` - 可观测性

### pkg/distributed/xdlock

**接口**：
- `Factory` - 锁工厂接口
  - `TryLock(ctx, key string, opts ...MutexOption) (LockHandle, error)` - 非阻塞获取锁
  - `Lock(ctx, key string, opts ...MutexOption) (LockHandle, error)` - 阻塞获取锁
  - `Close() error` - 关闭工厂
  - `Health(ctx context.Context) error` - 健康检查
- `LockHandle` - 锁句柄
  - `Unlock(ctx context.Context) error` - 释放锁
  - `Extend(ctx context.Context) error` - 续期
  - `Key() string` - 获取锁 Key
- `EtcdFactory` - etcd 锁工厂（扩展 Factory，暴露 Session）
- `RedisFactory` - Redis 锁工厂（扩展 Factory，暴露 Redsync）

**工厂函数**：
- `NewEtcdClient(config *EtcdConfig, opts ...EtcdClientOption) (*clientv3.Client, error)`
- `NewEtcdFactory(client *clientv3.Client, opts ...EtcdFactoryOption) (EtcdFactory, error)`
- `NewEtcdFactoryFromConfig(config, clientOpts, factoryOpts) (EtcdFactory, *clientv3.Client, error)`
- `NewRedisFactory(clients ...redis.UniversalClient) (RedisFactory, error)`

**锁选项**：
- `WithKeyPrefix(prefix string)` - Key 前缀（默认 "lock:"）
- `WithExpiry(d time.Duration)` - 锁过期时间（Redis，默认 8s）
- `WithTries(n int)` - 最大重试次数（Redis，默认 32）
- `WithRetryDelay(d time.Duration)` - 重试间隔（Redis，默认 200ms）
- `WithFailFast(b bool)` - 快速失败模式

### pkg/distributed/xcron

**接口**：
- `Scheduler` - 任务调度器
  - `AddFunc(spec string, cmd func(ctx) error, opts ...JobOption) (JobID, error)` - 添加函数任务
  - `AddJob(spec string, job Job, opts ...JobOption) (JobID, error)` - 添加 Job 任务
  - `Remove(id JobID)` - 移除任务
  - `Start()` - 启动调度（非阻塞）
  - `Stop() context.Context` - 优雅停止
  - `Cron() *cron.Cron` - 获取底层 cron 实例
  - `Entries() []cron.Entry` - 获取所有已注册任务
  - `Stats() *Stats` - 执行统计
- `Job` - 任务接口
  - `Run(ctx context.Context) error`
- `Locker` - 分布式锁接口
  - `TryLock(ctx, key string, ttl time.Duration) (LockHandle, error)`
- `LockHandle` - 锁句柄
  - `Unlock(ctx context.Context) error`
  - `Renew(ctx context.Context, ttl time.Duration) error`
  - `Key() string`
- `Hook` - 任务执行钩子
  - `BeforeJob(ctx, name string) context.Context`
  - `AfterJob(ctx, name string, duration time.Duration, err error)`

**工厂函数**：
- `New(opts ...SchedulerOption) Scheduler` - 创建调度器
- `NoopLocker() Locker` - 单副本空锁
- `NewRedisLocker(client redis.UniversalClient, opts ...RedisLockerOption) *RedisLocker`

**调度器选项**：
- `WithLocker(locker Locker)` - 设置默认锁
- `WithLogger(logger Logger)` - 设置日志
- `WithLocation(loc *time.Location)` - 设置时区
- `WithSeconds()` - 启用秒级精度

**任务选项**：
- `WithName(name string)` - 任务名称（用于锁 Key）
- `WithJobLocker(locker Locker)` - 单任务锁
- `WithLockTTL(ttl time.Duration)` - 锁超时
- `WithTimeout(timeout time.Duration)` - 执行超时
- `WithRetry(policy RetryPolicy)` - 重试策略
- `WithBackoff(policy BackoffPolicy)` - 退避策略
- `WithTracer(tracer Observer)` - 链路追踪
- `WithImmediate()` - 注册后立即执行
- `WithHook(hook Hook) / WithHooks(hooks ...Hook)` - 执行钩子

**适配类型**：
- `JobFunc func(ctx context.Context) error` - 函数适配 Job 接口
- `HookFunc{Before, After}` - 函数适配 Hook 接口

### pkg/config/xconf

**接口**：
- `Config` - 配置接口
  - `Client() *koanf.Koanf` - 获取底层 koanf 实例
  - `Unmarshal(path string, target any) error` - 反序列化
  - `MustUnmarshal(path string, target any)` - 反序列化（失败 panic）
  - `Reload() error` - 重新加载（仅文件模式）
  - `Path() string` - 配置文件路径
  - `Format() Format` - 配置格式
- `WatchConfig` - 带监听的配置接口（扩展 Config）
  - `Watch(callback WatchCallback, opts ...WatchOption) (*Watcher, error)`

**工厂函数**：
- `New(path string, opts ...Option) (Config, error)` - 从文件创建（自动识别格式）
- `NewFromBytes(data []byte, format Format, opts ...Option) (Config, error)` - 从字节创建
- `Watch(cfg Config, callback WatchCallback, opts ...WatchOption) (*Watcher, error)` - 创建文件监听

**配置格式**：
- `FormatYAML` - YAML 格式
- `FormatJSON` - JSON 格式

**选项函数**：
- `WithDelim(delim string) Option` - Key 分隔符（默认 "."）
- `WithTag(tag string) Option` - 结构体 Tag（默认 "koanf"）
- `WithDebounce(d time.Duration) WatchOption` - 监听防抖（默认 100ms）

### pkg/business/xauth

**接口**：
- `Client` - 认证服务客户端
  - `GetToken(ctx, tenantID string) (string, error)` - 获取 Token
  - `VerifyToken(ctx, token string) (*TokenInfo, error)` - 验证 Token
  - `GetPlatformID(ctx, tenantID string) (string, error)` - 获取平台 ID
  - `HasParentPlatform(ctx, tenantID string) (bool, error)` - 判断是否有父平台
  - `GetUnclassRegionID(ctx, tenantID string) (string, error)` - 获取未归类组 Region ID
  - `Request(ctx, req *AuthRequest) error` - 发送带认证的 HTTP 请求
  - `Close() error` - 关闭客户端
- `CacheStore` - 缓存存储接口
- `ContextClient` - 扩展接口（基于 context 的便捷方法）

**工厂函数**：
- `NewClient(cfg *Config, opts ...Option) (Client, error)`
- `MustNewClient(cfg *Config, opts ...Option) Client`
- `NewRedisCacheStore(client redis.UniversalClient, opts ...RedisCacheOption) *RedisCacheStore`
- `AsContextClient(c Client) ContextClient`

### pkg/debug/xdbg

**类型**：
- `Server` - 运行时调试服务（Unix Socket + SO_PEERCRED）

**工厂函数**：
- `New(opts ...Option) (*Server, error)`

### pkg/lifecycle/xrun

**接口**：
- `Service` - 服务接口（Run）
- `HTTPServerInterface` - HTTP 服务接口

**工厂函数**：
- `NewGroup(ctx context.Context, opts ...Option) (*Group, context.Context)`
- `Run(ctx context.Context, services ...func(ctx context.Context) error) error`
- `RunServices(ctx context.Context, services ...Service) error`
- `HTTPServer(server HTTPServerInterface, shutdownTimeout time.Duration, opts ...Option) func(ctx context.Context) error`

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

### pkg/util/xfile

**常量**：
- `DefaultDirPerm = 0750` - 目录默认权限

**函数**：
- `EnsureDir(filename string) error` - 确保文件父目录存在（权限 0750）
- `EnsureDirWithPerm(filename string, perm os.FileMode) error` - 指定权限确保目录
- `SanitizePath(filename string) (string, error)` - 路径安全检查与规范化
- `SafeJoin(base, path string) (string, error)` - 安全路径拼接（防止目录穿越）
- `SafeJoinWithOptions(base, path string, opts SafeJoinOptions) (string, error)` - 带符号链接解析的安全拼接

### pkg/util/xjson

**函数**：
- `Pretty(v any) string` - 格式化 JSON 输出（用于日志和调试）

### pkg/util/xkeylock

**接口**：
- `KeyLock` - 基于 key 的进程内互斥锁（Acquire/TryAcquire/Close）
- `Handle` - 锁句柄（Unlock/Key）

**工厂函数**：
- `New(opts ...Option) KeyLock`

### pkg/util/xlru

**类型**：
- `Cache[K, V]` - 泛型 LRU 缓存（TTL 支持）

**工厂函数**：
- `New[K, V](cfg Config, opts ...Option[K, V]) (*Cache[K, V], error)`

### pkg/util/xnet

**核心函数**：
- `ParseRange(s string) (netipx.IPRange, error)` - 解析 IP/CIDR/掩码/范围
- `ParseRanges(strs []string) (*netipx.IPSet, error)` - 批量解析
- `AddrFromUint32/AddrToUint32` - IPv4 与 uint32 互转
- `AddrFromBigInt/AddrToBigInt` - 与 big.Int 互转
- `FormatFullIP/ParseFullIP` - 全长格式化与解析
- `WireRangeFrom/WireRangesToSet` - 序列化工具

### pkg/util/xpool

**类型**：
- `WorkerPool[T]` - 泛型 Worker Pool

**工厂函数**：
- `NewWorkerPool[T](workers, queueSize int, handler func(T)) *WorkerPool[T]`

### pkg/util/xproc

**函数**：
- `ProcessID() int` - 返回当前进程 ID
- `ProcessName() string` - 返回当前进程名称（不含路径）

### pkg/util/xsys

**函数**：
- `SetFileLimit(limit uint64) error` - 设置进程最大打开文件数（RLIMIT_NOFILE）
- `GetFileLimit() (soft, hard uint64, err error)` - 查询进程最大打开文件数

### pkg/util/xutil

**函数**：
- `If[T any](cond bool, trueVal, falseVal T) T` - 类型安全的三目运算符（注意：两个值均会求值，不会短路）

---

## 稳定性承诺

- **Stable** 包：遵循语义版本控制，不做破坏性变更
- **Beta** 包：API 可能调整，会提前在 CHANGELOG 中说明
- **Alpha** 包：实验性，API 可能随时变化
