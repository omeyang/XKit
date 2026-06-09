---
package: pkg/util/xpool
stability: Stable
coverage: 100.0%
tags: [util, worker-pool, concurrency, generic]
related:
  - ../../06-patterns/08-worker-pool.md
  - ../../05-concepts/08-panic-recover-discipline.md
---

# pkg/util/xpool

泛型 Worker Pool。固定 worker 数 + 有界队列 + 优雅关闭。

## 用途

- 限制并发：固定 N 个 worker 处理任务流
- 解耦生产消费：队列缓冲突发负载
- 任务级 panic 隔离：单任务 panic 不影响其他 worker

## 不适用场景

- 跨进程任务分发：用消息队列（[xkafka](../08-mq/01-xkafka.md) / [xpulsar](../08-mq/02-xpulsar.md)）
- 任务长尾且需要观测：本包刻意不内置可观测性（util 包约束，见 ADR-0007）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xpool"

p, err := xpool.New[string](
    8,    // workers
    100,  // queue size
    func(task string) {
        process(task)
    },
    xpool.WithLogger(slog.Default()),
)
if err != nil { /* ... */ }
defer p.Shutdown(context.Background())

if err := p.Submit("hello"); err != nil {
    // ErrQueueFull / ErrPoolStopped
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Pool[T]` | 泛型 Pool |
| `New[T](workers, queueSize, handler, opts...) (*Pool[T], error)` | 构造 |
| `Submit(task T) error` | 非阻塞提交 |
| `Shutdown(ctx) error` | 带超时优雅关闭 |
| `Close() error` | 等所有任务完成（=Shutdown(Background)）|
| `Done() <-chan struct{}` | 所有 worker 退出后关闭的通道 |
| `Workers / QueueSize / QueueLen` | 状态查询 |
| `WithLogger(*slog.Logger)` | 自定义日志 |
| `ErrNilHandler / ErrPoolStopped / ErrQueueFull / ErrInvalidWorkers / ErrInvalidQueueSize` | 错误变量 |

## 设计要点

- **`submitMu` RWMutex**：保护 queue 发送 vs Close；write lock 等所有 in-flight Submit
- **`safeHandle` 嵌套 recover**：外层 recover 防 handler panic；内层 recover 防 logger 自身 panic 二次崩溃
- **上限**：`maxWorkers = 1 << 16`，`maxQueueSize = 1 << 24`
- **非阻塞提交**：队列满 → 返 `ErrQueueFull`，由调用方决策（设计决策，见 doc.go:49-52）
- **handler 无 context**：刻意不传 ctx，由 handler 自己捕获（设计决策）
- **`QueueLen` 无锁**：`len(channel)` Go 规范保证并发安全
- **`Pool` 值复制守护**：含 `sync.WaitGroup` 的 `noCopy`，`go vet copylocks` 守护；`New` 返回 `*Pool` 指针

## 相关

- 概念：
  - [Panic recover 纪律](../../05-concepts/08-panic-recover-discipline.md)
- 模式：
  - [Worker Pool 模式](../../06-patterns/08-worker-pool.md)
- ADR：
  - [0007 util 包不内置可观测性](../../01-decisions/0007-util-packages-no-builtin-observability.md)
- API：[api.md#pkgutilxpool](../../03-conventions/01-api.md#pkgutilxpool)
