---
package: pkg/resilience/xlimit
stability: Beta
coverage: 95.3%
tags: [resilience, rate-limit, token-bucket, redis]
---

# pkg/resilience/xlimit

分布式限流器。Token Bucket 算法 + Redis 后端。

## 用途

- API 限流（每秒 N 个请求）
- 多副本协同（避免每个 pod 独立计数）

## 不适用场景

- 单副本：用 `golang.org/x/time/rate` 更轻量

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/resilience/xlimit"

l := xlimit.NewTokenBucket(redisClient, "api:foo",
    xlimit.WithRate(100),               // 100 tokens/sec
    xlimit.WithCapacity(200),
)

if !l.Allow(ctx) {
    return errors.New("rate limit exceeded")
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Limiter` interface | `Allow(ctx) bool` / `Wait(ctx) error` |
| `NewTokenBucket(client, key, opts...)` | Redis 后端 |
| `WithRate / WithCapacity` | 配置 |

## 设计要点

- **Lua 原子**：避免 race
- **告警与文档**：对抗审查加强了 Burst/Rate 边界的告警与文档

## 相关

- API：[api.md#pkgresiliencexlimit](../../03-conventions/01-api.md#pkgresiliencexlimit)
