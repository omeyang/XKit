---
package: pkg/lifecycle/xhealth
stability: Stable
coverage: 92.4%
tags: [lifecycle, health, kubernetes]
related:
  - ../../05-concepts/08-panic-recover-discipline.md
---

# pkg/lifecycle/xhealth

Kubernetes 健康探针。HTTP 监听暴露 `/healthz`、`/readyz`、`/startupz` 端点，可注册多检查器。

## 用途

- K8s liveness / readiness / startup probe
- 检查内部依赖（DB ping / DNS / 自定义）

## 使用入口

示例文件中的 `Example`、`ExampleHealth_CheckReadiness` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/lifecycle/xhealth` 查看。

[包说明](../../../pkg/lifecycle/xhealth/doc.go) · [使用示例](../../../tests/unit/pkg/lifecycle/xhealth/example_test.go)

## 设计要点

- **`statusListener` 回调补 recover** 防御
- **缓存 TTL**：避免高频探针打爆下游
- **detail 仅在带 query 参数时返**：避免泄漏内部信息

## 相关

- 概念：[Panic recover 纪律](../../05-concepts/08-panic-recover-discipline.md)
- API：[api.md#pkglifecyclexhealth](../../03-conventions/01-api.md#pkglifecyclexhealth)
