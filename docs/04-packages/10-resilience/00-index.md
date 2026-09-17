---
title: Resilience 包
tags: [moc, packages, resilience]
---

# Resilience 包

韧性模式三件套：熔断 / 重试 / 限流。配合 Context 超时即可覆盖大部分场景。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xbreaker](01-xbreaker.md) | 熔断器（基于 sony/gobreaker） | Stable | 99.4% |
| [xretry](03-xretry.md) | 重试策略（指数退避，可中断） | Stable | 96.3% |
| [xlimit](02-xlimit.md) | 分布式限流器（Token Bucket，Redis 后端） | Stable | 95.3% |

## 组合使用

典型外部调用栈：
```
Context（超时）→ xbreaker（熔断）→ xretry（重试）→ 业务调用
```

或加上限流：
```
xlimit（速率限制）→ xbreaker → xretry → 调用
```

## 相关模式

- [熔断器模式](../../06-patterns/03-circuit-breaker.md)
- [API 清单 Resilience 段](../../03-conventions/01-api.md)
