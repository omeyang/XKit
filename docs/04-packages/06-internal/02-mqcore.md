---
package: internal/mqcore
stability: Internal
coverage: 100.0%
tags: [internal, mq, consume-loop]
related:
  - ../../06-patterns/05-fail-fast-consume-loop.md
---

# internal/mqcore

MQ 通用消费循环。`xkafka` 与 `xpulsar` 共享底层。**Fail-Fast 设计**：handler panic 刻意不补 recover。

## 用途

- `RunConsumeLoop(ctx, handler)` 给 xkafka / xpulsar 用
- 统一消费循环骨架（ctx 监听、消息分发、错误处理）

## 设计要点

- **Fail-Fast**：handler panic 不补 recover，让其暴露
- **理由**：消息处理 handler 是业务方的代码，panic 表明业务 bug；自动 recover 会掩盖问题
- **对比**：xpool / xhealth / xelection / xconf 这些**包内拥有 goroutine** 的场景必须 recover

## 相关

- 模式：[Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)
- 包：[xkafka](../08-mq/01-xkafka.md) / [xpulsar](../08-mq/02-xpulsar.md)
