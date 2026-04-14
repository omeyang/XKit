# 03 · 业务场景

XKit 是库（非应用），业务场景描述的是**被依赖方（下游业务服务）**的典型使用形态。

## 定位

为 Go 后端服务提供**标准化基础设施**，让业务代码不再重复实现：

- 结构化日志、链路追踪、指标、采样、日志轮转
- 熔断、重试、限流
- Redis / etcd / MongoDB / ClickHouse 客户端封装
- Kafka / Pulsar 生产消费（含 DLQ、OTel 贯穿）
- 分布式锁、分布式定时任务
- 配置加载（koanf）、认证客户端
- 进程生命周期（信号 + errgroup）、调试端点
- 通用工具（LRU、Worker Pool、IP/MAC、文件路径、泛型）

## 典型使用形态

### 业务服务初始化

```
main() → xrun.Run(ctx,
    xlog.Init(...),
    xmetrics.Init(...),
    xtrace.Init(...),
    业务 Server.Start,
)
```

### 请求链路

```
gRPC/HTTP 请求
  → xtrace 拦截器（注入 traceparent）
  → xtenant 中间件（提取租户）
  → xctx 增强 context
  → 业务 Handler
    → xbreaker/xretry/xlimit 包装外部调用
    → xcache/xmongo/xclickhouse 访问存储
    → xkafka/xpulsar 发消息（traceparent 贯穿）
```

### 后台任务

```
xcron（分布式调度）→ xdlock（抢占 leader）→ 业务 Job
```

## 业务边界

XKit **负责**：

- 基础设施客户端封装、可观测性贯穿、弹性模式实现
- 跨请求/跨服务的身份与上下文传递约定
- 资源与生命周期管理

XKit **不负责**：

- 具体业务领域模型与领域服务
- HTTP/gRPC 路由与 API 契约（由业务服务定义）
- 业务数据库 Schema 与迁移
- 前端/BFF 层逻辑
- 企业级框架能力（DI 容器、规则引擎、工作流引擎）

## 稳定性边界

公开包稳定性见 `README.md` 与 [06-progress.md](06-progress.md)。`Alpha/Beta` 标记的包 API 可能调整；`Stable` 包遵循语义化版本。
