---
package: pkg/observability/xsampling
stability: Stable
coverage: 97.9%
tags: [observability, sampling]
---

# pkg/observability/xsampling

采样策略。公开 API 已冻结（Stable）。

## 用途

- 高吞吐服务限制 trace/log 采样率
- 错误请求强制全采，正常请求按比例采

## 快速上手

```go
rate, _ := xsampling.NewRateSampler(0.1)  // 10% 采样
count, _ := xsampling.NewCountSampler(100) // 每 100 次采一次
combined, _ := xsampling.Any(rate, count)  // OR 组合
_ = combined.ShouldSample(ctx)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Sampler` interface | `ShouldSample(ctx context.Context) bool` |
| `Always() / Never()` | 全采 / 全不采 |
| `NewRateSampler / NewCountSampler / NewKeyBasedSampler` | 基础策略，均返回 `(T, error)` |
| `All / Any / NewCompositeSampler` | 组合策略（`ModeAND` / `ModeOR`），均返回 `(*CompositeSampler, error)` |

## 设计要点

- **策略组合固定**：`CompositeSampler` 按传入顺序短路求值，任一子策略返回 true 即采样
- **可拼接**：基础策略 + 组合 sampler

## 相关

- API：[api.md#pkgobservabilityxsampling](../../03-conventions/01-api.md#pkgobservabilityxsampling)
