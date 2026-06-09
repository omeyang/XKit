---
package: pkg/observability/xsampling
stability: Alpha
coverage: 97.9%
tags: [observability, sampling]
---

# pkg/observability/xsampling

采样策略。**Alpha**：API 可能调整。

## 用途

- 高吞吐服务限制 trace/log 采样率
- 错误请求强制全采，正常请求按比例采

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/observability/xsampling"

s := xsampling.NewRatioSampler(0.1)  // 10% 采样
if s.ShouldSample(ctx) {
    // 采样路径
}

// 组合策略：错误强制采 + 正常按比例
combined := xsampling.NewCompositeSampler(
    xsampling.ErrorBiased(),
    xsampling.RatioSampler(0.05),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Sampler` interface | `ShouldSample(ctx) bool` |
| `NewRatioSampler / NewErrorBiased / NewCompositeSampler` | 策略 |

## 设计要点

- **Alpha 标识**：策略组合方式可能调整
- **可拼接**：基础策略 + 组合 sampler

## 相关

- API：[api.md#pkgobservabilityxsampling](../../03-conventions/01-api.md#pkgobservabilityxsampling)
