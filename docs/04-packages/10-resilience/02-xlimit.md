---
package: pkg/resilience/xlimit
stability: Stable
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

## 使用入口

示例文件中的 `Example_tenantRateLimit`、`Example_localLimiter` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/resilience/xlimit` 查看。

[包说明](../../../pkg/resilience/xlimit/doc.go) · [使用示例](../../../tests/unit/pkg/resilience/xlimit/example_test.go)

## 设计要点

- **Lua 原子**：避免 race

## 相关

- API：[api.md#pkgresiliencexlimit](../../03-conventions/01-api.md#pkgresiliencexlimit)
