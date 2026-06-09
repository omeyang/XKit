---
title: 术语表
tags: [glossary]
---

# 术语表

XKit 文档与对抗审查中频繁出现的简写与术语。

## 对抗审查相关

| 术语 | 含义 |
|---|---|
| **CA** | Claude A：对抗审查中的第一路 Claude 审查 |
| **CB** | Claude B：第二路 Claude（不同视角/prompt） |
| **Codex A** | OpenAI Codex 第一组 |
| **Codex B** | OpenAI Codex 第二组 |
| **CC** | Claude 交叉裁决（Cross-Check）：判定其他路发现的真伪 |
| **FG-H** | Finding Grade - High：高严重发现（race / NPE / data loss） |
| **FG-M** | Finding Grade - Medium：中严重（错误处理瑕疵、文档不准） |
| **FG-L** | Finding Grade - Low：低严重（命名、格式） |
| **FG-S** | Finding Grade - Style/Security：样式或安全独立汇总 |
| **FP** | False Positive：误报（不是真问题） |
| **slot** | 对抗审查 cron 的轮转位（0-7 / 0-14 等），对应固定包映射 |
| **a / b / c** | CC 裁决标签：a=真问题 / b=FP / c=不确定 |

## Go 语言术语

| 术语 | 含义 |
|---|---|
| **typed-nil** | `(*T)(nil)` 装箱为接口后接口非 nil 的陷阱（详见 [Typed-nil 陷阱](05-concepts/10-typed-nil-trap.md)） |
| **TOCTOU** | Time-Of-Check to Time-Of-Use：检查与使用之间的并发窗口 |
| **NX** | Redis SET NX：仅当 key 不存在时设置 |
| **RWMutex** | `sync.RWMutex`：读写分离的互斥锁 |
| **goleak** | `go.uber.org/goleak`：goroutine 泄漏检测库 |
| **errgroup** | `golang.org/x/sync/errgroup`：协调多 goroutine 的错误传播 |
| **bufconn** | `google.golang.org/grpc/test/bufconn`：内存 gRPC 测试通道 |

## XKit 特有术语

| 术语 | 含义 |
|---|---|
| **MOC** | Map of Content：领域索引文档（每个分区有一个 00-index.md） |
| **ADR** | Architecture Decision Record：架构决策记录（`docs/01-decisions/`） |
| **Fallback** | 降级策略：xsemaphore 的 `FallbackOpen` / `FallbackClosed` |
| **Lua + Fallback** | xsemaphore 在 Redis 失联时降级到本地信号量 |
| **Hash tag** | Redis 集群兼容用：`{resource}:` 前缀保证同槽 |
| **Informer** | etcd 仿 K8s client-go 的 list+watch 缓存（[xetcd](04-packages/12-storage/03-xetcd.md)） |
| **DLQ** | Dead Letter Queue：死信队列（[xkafka](04-packages/08-mq/01-xkafka.md) / [xpulsar](04-packages/08-mq/02-xpulsar.md)） |
| **Cache-Aside** | 缓存模式（详见 [Cache-Aside 模式](06-patterns/02-cache-aside.md)） |
| **Singleflight** | 同 key 并发去重（详见 [Singleflight 模式](06-patterns/07-singleflight.md)） |
| **Fail-Fast 消费** | MQ handler panic 不补 recover（详见 [模式](06-patterns/05-fail-fast-consume-loop.md)） |

## 工程约定术语

| 术语 | 含义 |
|---|---|
| **记账式表述** | 文档禁止的写法（"以前 A → 后来 B → 现在 C"）；由 `task docs-ledger-check` 强制 |
| **设计决策注释** | 代码里 `// 设计决策:` 前缀，解释非显然的取舍 |
| **接口由使用方定义** | 接口在调用方包内定义，不发布伞型接口（[ADR-0002](01-decisions/0002-interface-defined-by-consumer.md)） |
| **构造函数返 error** | 不 panic，统一错误传播（[ADR-0001](01-decisions/0001-constructor-returns-error-not-panic.md)） |

## 错误处理术语

| 术语 | 含义 |
|---|---|
| **`%w`** | `fmt.Errorf` 包装错误保留链 |
| **`%v`** | 切断错误链（在抽象边界使用） |
| **双 `%w`** | Go 1.20+ 多错误包装：`%w: %w` |
| **`errors.Join`** | 多个独立错误聚合 |
| **哨兵值** | `var ErrXxx = errors.New(...)`：可被 `errors.Is` 判定 |
| **comma-ok** | 类型断言强制 `v, ok := x.(T)` 形式（lint 强制） |

## 性能术语

| 术语 | 含义 |
|---|---|
| **benchmem** | `go test -bench=. -benchmem`：基准测试含内存分配 |
| **alloc** | allocation：堆分配次数（越少越好） |
| **escape analysis** | 逃逸分析：变量是否分配到堆 |
| **sync.Pool** | 对象池：复用临时对象，减少 GC 压力 |
| **Singleflight** | 同 key 并发去重，参见 [模式](06-patterns/07-singleflight.md) |

## 相关

- [文档索引](00-index.md)
- [包索引](04-packages/00-index.md)
- [概念笔记 MOC](05-concepts/00-index.md)
- [设计模式 MOC](06-patterns/00-index.md)
