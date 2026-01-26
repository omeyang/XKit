# Resilience 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/resilience/
├── xretry/    # 重试策略，基于 avast/retry-go/v5
└── xbreaker/  # 熔断器，基于 sony/gobreaker/v2

internal/
└── （相关内部实现）
```

## 模块职责

**设计原则**：
- 接口驱动设计（依赖倒置）
- 类型别名暴露底层库完整能力
- 配置与执行分离

**包职责**：
- **xretry**：RetryPolicy/BackoffPolicy 接口，支持 Retryer 和 retry-go 两种风格，错误分类（Permanent/Temporary/Unrecoverable）
- **xbreaker**：TripPolicy 熔断策略，状态机（Closed/Open/HalfOpen），可与 xretry 组合（BreakerRetryer）

**包间关系**：
- xbreaker 可与 xretry 组合使用
- xmq 的 DLQ 功能依赖 xretry 接口
