---
title: 概念笔记 MOC
tags: [moc, concepts]
---

# 概念笔记（MOC）

跨包共享的核心概念。每篇是 atomic 笔记，由多个 [包文档](../04-packages/00-index.md) 引用。

## 错误与异常

| 笔记 | 摘要 |
|---|---|
| [错误处理策略](02-error-handling-policy.md) | `%w` 跨包、`%v` 抽象边界、双 `%w` 多 cause、`errors.Join` 何时用 |
| [Typed-nil 陷阱](10-typed-nil-trap.md) | `(*T)(nil)` 装箱后非 nil interface 的陷阱；如何防御 |
| [Panic recover 纪律](08-panic-recover-discipline.md) | 何时 recover、何时刻意 fail-fast；嵌套 recover 防 logger 二次崩 |

## Context

| 笔记 | 摘要 |
|---|---|
| [Nil context 防御](06-nil-context-defense.md) | 入口统一 ErrNilContext；与 panic 路径的区分；Close 例外 |

## 接口与扩展

| 笔记 | 摘要 |
|---|---|
| [接口由使用方定义](04-interface-by-consumer.md) | 接口定义在调用方（按需），实现方不发布"伞型"接口 |
| [函数选项模式](03-functional-options.md) | `WithXxx` 选项链；为何不用 struct config |

## 并发与测试

| 笔记 | 摘要 |
|---|---|
| [并发安全](01-concurrency-safety.md) | goroutine 生命周期、退出协议、共享资源保护范式 |
| [测试模式](09-testing-patterns.md) | 表驱动、fuzz、goleak、testcontainers、golden files |
| [Mock 隔离](05-mock-isolation.md) | mock 放 `<pkg>mock/` 子包，不拖低主包覆盖率 |

## 可观测性

| 笔记 | 摘要 |
|---|---|
| [可观测性栈](07-observability-stack.md) | slog（唯一日志）+ OTel（trace/metrics）+ W3C TraceContext |

## 相关

- [包索引](../04-packages/00-index.md)
- [设计模式 MOC](../06-patterns/00-index.md)
- [关键决策（ADR）](../01-decisions/00-index.md)
- [术语表](../07-glossary.md)
