# 模块审查：pkg/observability（可观测性）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署，遵循 OpenTelemetry 语义规范。

## 模块概览

```
pkg/observability/
├── xlog/       # 结构化日志，基于 log/slog 扩展
├── xmetrics/   # 可观测性接口（Observer/Span/Attr），OTel 默认实现
├── xtrace/     # HTTP/gRPC 链路追踪传播中间件
├── xsampling/  # 采样策略（比率/计数/概率/一致性）
└── xrotate/    # 日志文件轮转
```

包间关系：
- xlog 使用 xctx 提取追踪信息，使用 xrotate 进行轮转
- xmetrics 提供 Observer 接口，其他包可依赖
- xtrace 使用 xctx 存取追踪信息

---

## xlog：结构化日志

**职责**：对 Go 1.21+ `log/slog` 的封装，提供增值功能。

**关键文件**：
- `builder.go` - Builder 模式配置
- `enrich.go` - EnrichHandler，自动从 context 注入字段
- `level.go` - 动态级别调整
- `lazy.go` - 延迟求值函数
- `global.go` - 全局 Logger

**增值功能**：
- EnrichHandler：自动注入 trace_id, span_id, request_id, platform_id, tenant_id, tenant_name
- 动态级别：运行时通过 SetLevel 热更新
- 部署类型固定属性：Build 时注入，避免热路径检查
- 延迟求值：Lazy* 系列函数避免不必要计算
- 日志轮转：集成 xrotate

**API**：
```go
logger, cleanup, err := xlog.New().
    SetLevel(xlog.LevelInfo).
    SetFormat("json").
    SetRotation("/var/log/app.log", xrotate.WithMaxSize(100)).
    Build()
defer cleanup()

xlog.Info(ctx, "message", slog.String("key", "value"))
xlog.Debug(ctx, "expensive", xlog.Lazy("data", func() any { return compute() }))
```

**级别**：LevelDebug(-4)、LevelInfo(0)、LevelWarn(4)、LevelError(8)。无 Trace/Fatal（设计决策）。

---

## xmetrics：可观测性接口

**职责**：定义最小化可观测性接口，默认 OTel 实现。

**关键文件**：
- `observer.go` - Observer/Span/Attr 接口定义
- `otel.go` - OpenTelemetry 实现

**核心接口**：
```go
type Observer interface {
    Start(ctx context.Context, opts SpanOptions) (context.Context, Span)
}
type Span interface {
    End(result Result)
    SetAttribute(key string, value any)
}
```

**统一指标命名**：
- `xkit.operation.total` - 操作计数
- `xkit.operation.duration` - 操作耗时
- 属性：component / operation / status

**使用模式**：
```go
obs, _ := xmetrics.NewOTelObserver()
ctx, span := xmetrics.Start(ctx, obs, xmetrics.SpanOptions{
    Component: "xmongo",
    Operation: "find_page",
    Kind:      xmetrics.KindClient,
})
defer span.End(xmetrics.Result{Err: err})
```

---

## xtrace：链路追踪传播

**职责**：HTTP/gRPC 通信中链路追踪信息的提取和注入。

**关键文件**：
- `http.go` - HTTP 中间件和客户端注入
- `grpc.go` - gRPC 拦截器

**传播字段**：
- TraceID: 16 字节，W3C Trace Context
- SpanID: 8 字节，W3C Trace Context
- RequestID: 业务层面 UUID
- TraceFlags: 1 字节采样标志

**协议映射**：
| 字段 | HTTP Header | gRPC Metadata |
|------|-------------|---------------|
| TraceID | X-Trace-ID | x-trace-id |
| SpanID | X-Span-ID | x-span-id |
| RequestID | X-Request-ID | x-request-id |
| W3C | traceparent | traceparent |

**W3C Trace Context**：
- 格式：`{version}-{trace-id}-{parent-id}-{trace-flags}`
- 示例：`00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01`
- 版本 "ff" 保留，始终无效
- 未知版本按 version-00 解析

**解析优先级**：traceparent 优先于 X-Trace-ID

**Tracestate 设计决策**：
- 解析到 TraceInfo.Tracestate 字段
- 不自动存入 context，不自动传播
- 原因：厂商相关，盲目传递可能导致问题

---

## xsampling：采样策略

**职责**：提供统一的 Sampler 接口和多种采样策略。

**关键文件**：
- `sampler.go` - Sampler 接口
- `rate.go` - 比率采样
- `count.go` - 计数采样
- `keybased.go` - 基于 key 的一致性采样
- `composite.go` - 组合采样器

**采样器类型**：
| 采样器 | 用途 | 特点 |
|--------|------|------|
| Always() / Never() | 全采样/不采样 | 常量返回 |
| NewRateSampler(rate) | 固定比率 | 如 10% 采样率 |
| NewCountSampler(n) | 计数采样 | 每 n 个采样 1 个 |
| NewProbabilitySampler(p) | 概率采样 | 每次独立判断 |
| NewKeyBasedSampler(rate, keyFunc) | 一致性采样 | 同一 key 跨进程一致 |
| NewCompositeSampler(mode, ...) | 组合采样 | AND/OR 逻辑 |

**KeyBasedSampler 一致性**：
- 使用 xxhash（github.com/cespare/xxhash/v2）
- 同一 trace_id 在所有服务中决策一致
- 服务重启后行为不变

**性能指标**（参考）：
- KeyBasedSampler: ~30 ns/op, 0 allocs/op
- 随机源：math/rand/v2（Go 1.22+）

---

## xrotate：日志轮转

**职责**：日志文件轮转，接口导向设计。

**关键文件**：
- `rotator.go` - Rotator 接口定义
- `lumberjack.go` - 基于 lumberjack v2 的实现

**接口定义**：
```go
type Rotator interface {
    Write(p []byte) (n int, err error)  // 并发安全
    Close() error
    Rotate() error  // 手动轮转
}
```

**文件权限**：
- lumberjack 默认 0600
- 通过 WithFileMode 修改
- 权限通过写入后 chmod 实现（存在短暂时间窗口）

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**xlog**：
- EnrichHandler 是否正确处理 nil context？
- 动态级别调整是否线程安全？
- Lazy 函数在低级别禁用时是否真的零开销？（Enabled() 检查）
- SetOnError 回调是否会阻塞日志写入？
- 日志轮转时是否会丢失日志？

**xmetrics**：
- 接口是否足够最小化？
- OTel 实现是否正确使用 SDK？
- 指标命名是否一致？

**xtrace**：
- traceparent 解析是否完全符合 W3C 规范？
- HTTP Header 大小写处理（HTTP/2 强制小写）
- gRPC metadata key 是否全部小写？
- 服务 B 不使用 xtrace 时，trace_id 是否会丢失？

**xsampling**：
- KeyBasedSampler 跨进程一致性测试
- CountSampler 的原子操作是否正确？
- 组合采样器 AND/OR 逻辑是否正确？
- 重置配置时是否线程安全？

**xrotate**：
- 轮转期间 Write 是否阻塞？
- Close 后调用 Write 的行为？
- 文件权限时间窗口对安全敏感场景的影响？

**集成点**：
- xlog EnrichHandler 提取的字段是否与 xctx 定义一致？
- 字段命名是否统一？（trace_id vs traceId）
