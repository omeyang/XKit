# 变更日志

本文件记录 xkit 项目的重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [未发布]

### 新增

#### Context（上下文与身份管理）

- **pkg/context/xctx**: 上下文管理包
  - 部署信息（DeploymentType）传递
  - 身份信息（TraceID、SpanID、RequestID）传递
  - Platform 结构体批量读写

- **pkg/context/xtenant**: 多租户上下文传递
  - gRPC 元数据传递
  - HTTP Header 传递

- **pkg/context/xplatform**: 平台信息管理
  - 平台 ID 和类型管理
  - 上下文集成

- **pkg/context/xenv**: 环境变量管理
  - 部署类型检测
  - 环境变量安全读取

#### Observability（可观测性）

- **pkg/observability/xlog**: 基于 slog 的现代日志系统
  - 支持 JSON/Text 输出格式
  - Builder 模式配置
  - 全局 logger 和上下文增强
  - Lazy 延迟求值函数

- **pkg/observability/xrotate**: 日志轮转包
  - 基于 lumberjack 实现
  - 支持大小/时间轮转
  - 支持 gzip 压缩和自动清理

- **pkg/observability/xtrace**: 分布式追踪集成
  - gRPC 拦截器
  - HTTP 中间件

- **pkg/observability/xmetrics**: 可观测性抽象层
  - Observer/Span 接口
  - NoopObserver 实现
  - OTel 集成支持

- **pkg/observability/xsampling**: 采样决策包
  - 多种采样策略

#### Resilience（弹性与容错）

- **pkg/resilience/xretry**: 重试工具包
  - 指数退避策略
  - 可配置重试次数

- **pkg/resilience/xbreaker**: 熔断器
  - 基于状态机实现
  - 可配置阈值

#### Storage（数据存储）

- **pkg/storage/xetcd**: etcd 客户端封装
  - Client 封装：连接管理、健康检查、TLS 支持
  - KV 操作：Get/Put/Delete/List/Exists/Count
  - 带 TTL 的键值对支持（PutWithTTL）
  - Watch 功能：基于 Channel 的键值变化监听、前缀监听
  - 与 xdlock 分布式锁集成

- **pkg/storage/xcache**: 缓存抽象层
  - Memory/Redis 双实现
  - 分布式锁支持
  - Loader 模式（singleflight + 缓存击穿防护）

- **pkg/storage/xmongo**: MongoDB 客户端封装
  - 连接管理
  - 健康检查

- **pkg/storage/xclickhouse**: ClickHouse 客户端封装
  - 连接池管理
  - 健康检查

#### Distributed（分布式协调）

- **pkg/distributed/xdlock**: 分布式锁
  - Redis 分布式锁
  - etcd 分布式锁

- **pkg/distributed/xcron**: 分布式定时任务
  - 任务调度器
  - 分布式锁集成

#### MQ（消息队列）

- **pkg/mq/xkafka**: Kafka 客户端封装
  - 生产者/消费者封装
  - DLQ（死信队列）支持
  - OpenTelemetry 链路追踪集成
  - W3C Trace Context 标准实现

- **pkg/mq/xpulsar**: Pulsar 客户端封装
  - 生产者/消费者封装
  - DLQ（死信队列）支持
  - OpenTelemetry 链路追踪集成

#### Config（配置管理）

- **pkg/config/xconf**: 配置管理
  - 基于 koanf 实现
  - 支持多种配置源

#### Util（通用工具）

- **pkg/util/xfile**: 文件路径操作工具包
  - 安全的路径处理函数
  - 路径穿越检测

---

## 开发说明

### 集成测试

消息队列相关测试需要 `integration` build tag：

```bash
go test -tags=integration ./pkg/mq/...
```