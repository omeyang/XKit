---
title: 函数选项模式
tags: [concept, options, api-design]
related:
  - ../01-decisions/0003-functional-options-for-config.md
---

# 函数选项模式

## 规则

构造器接受 `...Option`，每个 Option 是 `func(*config)`。

## 标准形态

```go
type Option func(*options)

type options struct {
    timeout time.Duration
    logger  Logger
}

func WithTimeout(d time.Duration) Option {
    return func(o *options) { o.timeout = d }
}

func WithLogger(l Logger) Option {
    return func(o *options) { o.logger = l }
}

func New(arg string, opts ...Option) (*Foo, error) {
    o := &options{timeout: 30*time.Second}  // 默认
    for _, opt := range opts {
        opt(o)
    }
    if err := o.validate(); err != nil {
        return nil, err
    }
    return &Foo{opts: o}, nil
}
```

## 为何不用 struct config

```go
// 不推荐
func New(cfg Config) *Foo  // ← 调用方必须了解所有字段、零值含义
```

问题：
- 零值语义模糊（`Timeout: 0` 是"不超时"还是"未配置"？）
- 新增字段是破坏性变更（向后不兼容）
- 调用方写一堆零值字段

函数选项模式：
- 默认值显式在构造器里
- 新增 Option 不破坏现有代码
- 调用点只写关心的项

## 多组 Option：全局 vs 操作

XKit 中常见两层 Option：

```go
// 全局选项（与实例生命周期同步）
sem, _ := xsemaphore.New(client, "r",
    xsemaphore.WithLogger(logger),     // ← 全局
    xsemaphore.WithFallback(...),
)

// 操作选项（与单次调用同步）
permit, _ := sem.Acquire(ctx,
    xsemaphore.WithCapacity(10),       // ← 操作
    xsemaphore.WithTTL(30*time.Second),
)
```

类型分开：`Option` vs `AcquireOption` vs `QueryOption`，避免误用。

## 验证

构造器内统一调用 `o.validate()` 返错（[0001 构造函数返回 error](../01-decisions/0001-constructor-returns-error-not-panic.md)），不 panic。

## 反例：返回值 Option

```go
// 不推荐
func WithTimeout(d time.Duration) (Option, error)  // ← Option 不应返错
```

错误延迟到 `New` 调用时再统一抛，调用点更干净。

## 相关

- ADR：[0003 函数选项模式](../01-decisions/0003-functional-options-for-config.md)、[0001 构造函数返回 error](../01-decisions/0001-constructor-returns-error-not-panic.md)
- 几乎所有 XKit 包都用此模式
