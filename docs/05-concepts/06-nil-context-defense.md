---
title: Nil context 防御
tags: [concept, context, defense]
---

# Nil context 防御

## 规则

所有公开 API 入口对 nil ctx 必须**显式返错**，不允许 panic。错误统一为包级 `ErrNilContext`。

```go
package xsemaphore

var ErrNilContext = errors.New("xsemaphore: nil context")

func (s *semaphore) Acquire(ctx context.Context, opts ...AcquireOption) (Permit, error) {
    if ctx == nil {
        return nil, ErrNilContext
    }
    // ...
}
```

## 为什么不 panic

- **测试稳定性**：panic 会污染并发测试日志
- **接口一致**：业务方期待错误而非崩溃
- **错误链可追溯**：`errors.Is(err, xsemaphore.ErrNilContext)`

## 例外：Close

Close 方法允许接受 nil ctx（或刻意保留 `context.Background()` 路径）：
- Close 的语义是"释放资源"，不应因 ctx 缺失而失败
- 关闭操作通常发生在 panic 或异常路径，调用方可能没机会构造 ctx

xsemaphore 的 `Close(ctx)` 接口保留 ctx 参数仅为接口统一性，**实现内部不读 ctx**。

## 入口列表（应该做 nil ctx 检查）

通常需要的：
- `Acquire / Release / Extend / Query`（[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)）
- `Campaign / Resign`（[xelection](../04-packages/05-distributed/03-xelection.md)）
- `WarmupScripts`（[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)）
- HTTP/gRPC client `Do / request`（[xauth](../04-packages/01-business/01-xauth.md)）

通常**不需要**（Close 例外）：
- `Close`

## typed-nil context 问题

```go
var ctx context.Context = (*myCtx)(nil)  // typed-nil
if ctx == nil { ... }                    // ← false，进入主流程后 panic
```

XKit 设计决策：**不防御 typed-nil ctx**。理由：
- Go 标准库 context 也不防御
- typed-nil context 不是标准 Go 实践
- 防御会让所有入口都加反射检查，性能成本不值

## 相关

- 概念：[错误处理策略](02-error-handling-policy.md)、[Typed-nil 陷阱](10-typed-nil-trap.md)
- 包：[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)、[xelection](../04-packages/05-distributed/03-xelection.md)、[xauth](../04-packages/01-business/01-xauth.md)
