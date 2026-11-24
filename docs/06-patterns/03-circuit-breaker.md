---
title: Circuit Breaker 模式
tags: [pattern, resilience]
---

# Circuit Breaker

## 问题

下游服务异常时，请求堆积、上游线程耗尽、级联故障。

## 解法：三态机

```
        失败次数 ≥ 阈值
Closed ─────────────────→ Open
  ↑                         │
  │ 成功 ≥ MaxRequests       │ Timeout
  │                         ↓
Half-Open ←─────────────────┘
            试探请求成功
```

- **Closed**：正常通过
- **Open**：立即失败（不调下游）
- **Half-Open**：放少量请求试探，成功则 Closed，失败则回 Open

## XKit 实现

[xbreaker](../04-packages/10-resilience/01-xbreaker.md) 基于 `sony/gobreaker`。

## 使用熔断器

通过 `xbreaker.NewBreaker` 创建并复用熔断器。只返回错误的调用使用 `breaker.Do`，需要业务结果的调用使用包函数 `xbreaker.Execute`。连续失败阈值可用 `WithTripPolicy(NewConsecutiveFailures(n))` 配置。

[初始化、状态变化和带返回值的示例](../../tests/unit/pkg/resilience/xbreaker/example_test.go)

## 组合栈

典型外部调用栈：

```
Context (超时)
  → Circuit Breaker (熔断)
    → Retry (重试)
      → 业务调用
```

或加限流：

```
Rate Limit (限流)
  → Circuit Breaker
    → Retry
      → 业务调用
```

## 关键陷阱

### 1. 熔断不能替代超时

熔断器统计失败需要"何为失败"的定义。**调用必须有 ctx 超时**，否则慢失败把熔断器逼到 Open 时已经堆积大量请求。

### 2. 半开试探必须有上限

`WithMaxRequests` 控制 Half-Open 阶段试探请求数。试探失败应立即回 Open（不是再等次数）。

### 3. 保留业务错误

回调应返回下游调用的错误，以便熔断器按配置的策略统计调用结果。

## 相关

- 包：[xbreaker](../04-packages/10-resilience/01-xbreaker.md)、[xretry](../04-packages/10-resilience/03-xretry.md)、[xlimit](../04-packages/10-resilience/02-xlimit.md)
