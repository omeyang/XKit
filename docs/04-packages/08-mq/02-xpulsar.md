---
package: pkg/mq/xpulsar
stability: Stable
coverage: 99.5%
tags: [mq, pulsar, dlq, otel]
---

# pkg/mq/xpulsar

Pulsar 客户端。基于 `pulsar-client-go`，加 DLQ + OTel 链路追踪。

## 用途

- 多租户消息系统
- 延迟消息 / 定时消息
- 多订阅模式（Exclusive/Shared/Failover/KeyShared）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/mq/xpulsar"

c, _ := xpulsar.NewClient(xpulsar.Config{
    URL: "pulsar://localhost:6650",
})
defer c.Close()

p, _ := c.NewProducer("topic-1")
_ = p.Send(ctx, []byte(payload))

con, _ := c.NewConsumer(xpulsar.ConsumerConfig{
    Topic: "topic-1",
    Subscription: "my-sub",
    Type: xpulsar.SubscriptionShared,
})
_ = con.ConsumeLoop(ctx, handler)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Client / Producer / Consumer` | 三大类型 |
| `ConsumerConfig.Type` | Exclusive / Shared / Failover / KeyShared |
| `DLQConfig` | 死信队列 |
| `TracingClient` | OTel 装饰器 |

## 设计要点

- **覆盖率最高（99.5%）**：测试用例完整
- **OTel propagator nil 守卫**：与 xkafka 同样（OTelTracer 零值）

## 相关

- 模式：[Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)
- 内部：[mqcore](../06-internal/02-mqcore.md)
- API：[api.md#pkgmqxpulsar](../../03-conventions/01-api.md#pkgmqxpulsar)
