---
title: 错误处理策略
tags: [concept, error-handling]
related:
  - ../01-decisions/0004-error-wrapping-policy.md
---

# 错误处理策略

## 规则

| 情况 | 用 |
|---|---|
| 跨包传播底层错误 | `fmt.Errorf("...: %w", err)` |
| 跨过抽象边界（内部实现不让上层 `errors.Is` 到） | `fmt.Errorf("...: %v", err)` |
| 一个错误来自多个 cause | `fmt.Errorf("...: %w: %w", a, b)`（Go 1.20+）或 `errors.Join(a, b)` |
| 多个独立错误聚合（如批量操作部分失败） | `errors.Join(errs...)` |
| 入口校验失败 | 包级 `Err*` 哨兵值 + `errors.Is` 判定 |

## 何时 `%v` 而非 `%w`

`%v` 是**有意切断**错误链的工具。用在：

- **抽象边界**：内部实现细节不让上层依赖。如 `xid.ErrIDGenerationFailed` 包装内部 sonyflake 错误时用 `%v`（[详见 xsemaphore](../04-packages/05-distributed/04-xsemaphore.md#设计要点)）
- **依赖第三方错误类型可能变化**：用 `%v` 切断后调用方只能靠你的哨兵值判定

注释里应明确说明：
```go
// 设计决策: 用 %v 切断 sonyflake 内部错误链，
// 上层应通过 errors.Is(err, ErrIDGenerationFailed) 判定。
return fmt.Errorf("xid: %w: %v", ErrIDGenerationFailed, sfErr)
```

## 何时双 `%w`

Go 1.20 起 `fmt.Errorf` 支持多个 `%w`。典型用例：

```go
// xid DefaultMachineID 策略 5：hostname 失败 + IP 失败都要保留
return 0, fmt.Errorf("xid: machine id: %w: %w", hostnameErr, ipErr)
```

调用方用 `errors.Is(err, ErrHostname)` 或 `errors.Is(err, ErrIP)` 都能命中。

## `errors.Join` vs 双 `%w` 的取舍

| | `%w: %w` | `errors.Join` |
|---|---|---|
| 含义 | 一个错误，多原因 | 多个独立错误聚合 |
| 错误消息可读性 | 自定义模板 | 默认换行拼接 |
| 用例 | 多 fallback 都失败 | 批量操作部分失败 |

## 跨抽象的哨兵值 vs 类型断言

XKit 偏好**哨兵值 + `errors.Is`**：
```go
var ErrCapacityFull = errors.New("xsemaphore: capacity full")

if errors.Is(err, ErrCapacityFull) { ... }
```

而非类型断言：
```go
var capErr *CapacityError
if errors.As(err, &capErr) { ... }  // ← 较少用
```

理由：哨兵值跨版本更稳定，类型字段（如错误码、context 信息）改了不影响 `Is` 判定。

## 例外：gRPC 错误

gRPC 生态用 `status.Code(err)` / `status.FromError(err)` 检查，**不走 `errors.Is`**。因为 gRPC status 跨 wire 转换会丢失 wrapper 链。这是 gRPC 惯例。

## 相关

- ADR：[0004 错误包装策略](../01-decisions/0004-error-wrapping-policy.md)
- 概念：[Typed-nil 陷阱](10-typed-nil-trap.md)
- 包示例：[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md) / [xid](../04-packages/14-util/02-xid.md) / [xnet](../04-packages/14-util/07-xnet.md)
