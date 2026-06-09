---
title: Fail-Fast 消费循环模式
tags: [pattern, mq, panic, fail-fast]
---

# Fail-Fast 消费循环

## 决策

MQ 消费循环（[xkafka](../04-packages/08-mq/01-xkafka.md) / [xpulsar](../04-packages/08-mq/02-xpulsar.md) / [mqcore](../04-packages/06-internal/02-mqcore.md)）的 handler panic **刻意不补 recover，让进程崩**。

## 与其他场景的对照

| 场景 | 是否 recover | 理由 |
|---|---|---|
| [xpool](../04-packages/14-util/08-xpool.md) worker | ✅ recover | worker 是包内 goroutine 资产，崩了整个 Pool 失效 |
| [xhealth](../04-packages/07-lifecycle/01-xhealth.md) statusListener | ✅ recover | 健康探针不能因业务回调 panic 而拒绝服务 |
| [xelection](../04-packages/05-distributed/03-xelection.md) observe | ✅ recover | 选举内部 goroutine，崩了影响主备切换 |
| [xconf](../04-packages/02-config/01-xconf.md) onChange | ✅ recover | 配置回调，崩了影响后续变更 |
| [mqcore](../04-packages/06-internal/02-mqcore.md) RunConsumeLoop | ❌ **不 recover** | handler 是业务代码，panic 表明业务 bug |
| [xkafka.ConsumeLoop](../04-packages/08-mq/01-xkafka.md) | ❌ **不 recover** | 同上 |

## 为什么 MQ 消费不 recover

1. **业务代码 panic 是真 bug**：自动 recover 会掩盖问题
2. **K8s 重启更可靠**：进程崩 → K8s 重启 → 重新订阅 → 消息会重投（at-least-once）
3. **MQ 自带重试机制**：消息未 ack 会重投，自动 recover 反而让消息丢失（offset 已提交）

## 与 K8s 配合

```yaml
restartPolicy: Always
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
```

进程崩 → K8s 重启 → 业务重连 MQ → 重投未 ack 消息。

## handler 内部允许局部 recover

如果业务方想容忍单条消息的非致命错误（如解析失败），可以在 handler **内部** recover：

```go
func handler(ctx context.Context, msg *xkafka.Message) error {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("handler panic", "panic", r, "msg", msg.Key)
            // 决策：返 nil 则 ack（丢消息）；返错则 DLQ
        }
    }()
    return doWork(ctx, msg)
}
```

这是**业务侧选择**，库层不强制。

## 相关

- 包：[mqcore](../04-packages/06-internal/02-mqcore.md)、[xkafka](../04-packages/08-mq/01-xkafka.md)、[xpulsar](../04-packages/08-mq/02-xpulsar.md)
- 概念：[Panic recover 纪律](../05-concepts/08-panic-recover-discipline.md)
