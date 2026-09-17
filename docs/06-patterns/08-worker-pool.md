---
title: Worker Pool 模式
tags: [pattern, concurrency, pool]
---

# Worker Pool

## 问题

- 限制并发数（避免无界 goroutine 爆）
- 解耦生产/消费节奏
- 优雅关闭管理

## XKit 实现

[xpool](../04-packages/14-util/08-xpool.md) 提供泛型 Worker Pool。

## 模板

```go
p, err := xpool.New[Job](
    8,    // workers
    100,  // queue size
    func(j Job) {
        process(j)
    },
    xpool.WithLogger(slog.Default()),
)
if err != nil { return err }
defer p.Shutdown(context.Background())

// 生产
for _, j := range jobs {
    if err := p.Submit(j); err != nil {
        // ErrQueueFull 时丢弃 / 退避 / 退到 fallback
    }
}
```

## 选型

| 需求 | 用 |
|---|---|
| 任务流处理 + 限并发 | [xpool](../04-packages/14-util/08-xpool.md) |
| 长任务 + 可中断 | 直接 errgroup |
| 跨进程任务分发 | [xkafka](../04-packages/08-mq/01-xkafka.md) / [xpulsar](../04-packages/08-mq/02-xpulsar.md) |

## 关键设计要点

### 1. 有界 vs 无界队列

XKit 强制有界（`queueSize`）。无界队列 = OOM 风险。

### 2. 非阻塞 Submit

`Submit` 满了直接返 `ErrQueueFull`。**调用方决策**：丢弃、退避、降级、拒绝请求。

### 3. handler 不传 ctx

[xpool](../04-packages/14-util/08-xpool.md) 刻意设计：handler 不接受 ctx。理由：
- handler 是业务代码，ctx 应该由业务方在闭包里捕获
- 简化 API（不强制每个 handler 处理 ctx）

### 4. Panic 隔离

`safeHandle` 双层 recover（详见 [Panic recover 纪律](../05-concepts/08-panic-recover-discipline.md)）：
- 外层 recover handler panic
- 内层 recover logger 自身 panic

### 5. submitMu + closed atomic.Bool

`Submit` 用 RWMutex `RLock` + closed 检查；`Shutdown` 用 `WLock` 等所有 in-flight Submit 完成后再标 closed。避免 close-of-closed channel。

## 相关

- 包：[xpool](../04-packages/14-util/08-xpool.md)
- 概念：[并发安全](../05-concepts/01-concurrency-safety.md)、[Panic recover 纪律](../05-concepts/08-panic-recover-discipline.md)
- ADR：[0007 util 不内置可观测性](../01-decisions/0007-util-packages-no-builtin-observability.md)
