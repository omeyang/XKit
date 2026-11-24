---
package: pkg/resilience/xbreaker
stability: Stable
coverage: 99.4%
tags: [resilience, circuit-breaker]
related:
  - ../../06-patterns/03-circuit-breaker.md
---

# pkg/resilience/xbreaker

熔断器。基于 `sony/gobreaker`，提供状态机封装与可观测性钩子。

## 用途

- 防止级联故障：下游异常时快速失败，避免堆积
- 给下游恢复时间

## 使用入口

示例文件中的 `ExampleNewBreaker`、`ExampleExecute` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/resilience/xbreaker` 查看。

[包说明](../../../pkg/resilience/xbreaker/doc.go) · [使用示例](../../../tests/unit/pkg/resilience/xbreaker/example_test.go)

## 设计要点

- **sony/gobreaker**：成熟库，不自造轮子
- **三态机**：Closed / Open / Half-Open

## 相关

- 模式：[Circuit Breaker](../../06-patterns/03-circuit-breaker.md)
- API：[api.md#pkgresiliencexbreaker](../../03-conventions/01-api.md#pkgresiliencexbreaker)
