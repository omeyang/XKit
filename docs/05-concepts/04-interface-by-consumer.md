---
title: 接口由使用方定义
tags: [concept, interface, design]
related:
  - ../01-decisions/0002-interface-defined-by-consumer.md
---

# 接口由使用方定义

## 规则

接口定义在**调用方**的包里，而不是实现方的包里。实现方不发布"伞型"接口。

## 反例

```go
// xkafka 内部
package xkafka

type Producer interface {
    Produce(ctx, topic, payload []byte) error
    Close() error
}

func NewProducer() Producer { ... }  // ← 伞型接口
```

业务方导入 xkafka 后被绑定到这个接口（哪怕只用了 Produce）。

## 正例

```go
// 业务方 myservice 包
package myservice

type messageProducer interface {  // ← 调用方定义,刚好够用
    Produce(ctx, topic, payload []byte) error
}

type EventService struct {
    p messageProducer
}

func NewEventService(p messageProducer) *EventService { ... }
```

调用方传入：
```go
kafkaProducer, _ := xkafka.NewProducer(cfg)  // 实现方返结构体指针
svc := myservice.NewEventService(kafkaProducer)
```

## XKit 实现层返结构体

XKit 各包的 `New(...)` 返回结构体指针（如 `*xsemaphore.semaphore` 实现 Semaphore 接口）：
- 接口可以由调用方定义最小化版本
- 也可以直接用 XKit 提供的接口（如调用方不想自己定义）

## 例外

部分包内部 goroutine 间通信仍需要接口（如 [xelection.Election](../04-packages/05-distributed/03-xelection.md)），这些是**包内**接口，不视为对外伞型。

## 测试套件如何 mock

业务方在自己的测试里：
```go
type mockProducer struct{ calls []string }
func (m *mockProducer) Produce(ctx, t, p []byte) error { ... }

func TestEventService(t *testing.T) {
    svc := myservice.NewEventService(&mockProducer{})
    // ...
}
```

不需要 mockgen，因为接口在调用方自己手里。

## XKit 提供的 mock 子包

对于复杂接口（[xsemaphore.Semaphore](../04-packages/05-distributed/04-xsemaphore.md)），XKit 提供 mockgen 生成的 mock 子包（[xsemaphoremock](../04-packages/13-testkit/00-index.md)），但这是"加分项"，**不应该**强制业务方使用。

## 相关

- ADR：[0002 接口由使用方定义](../01-decisions/0002-interface-defined-by-consumer.md)
- 概念：[Mock 隔离](05-mock-isolation.md)
- 包：所有 pkg 都遵循此原则
