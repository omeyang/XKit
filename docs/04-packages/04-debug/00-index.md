---
title: Debug 包
tags: [moc, packages, debug]
---

# Debug 包

运行时调试服务。通过 Unix Socket 暴露内省接口，避免 HTTP 调试端口被外部访问。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xdbg](01-xdbg.md) | Unix Socket 调试服务（goroutine/heap/stack 等） | Beta | 90.5% |

## 配套 CLI

- `cmd/xdbgctl`：命令行客户端（覆盖率 91.1%）

## 相关

- [概念：可观测性栈](../../05-concepts/07-observability-stack.md)
- [API 清单 xdbg 段](../../03-conventions/01-api.md#pkgdebugxdbg)
