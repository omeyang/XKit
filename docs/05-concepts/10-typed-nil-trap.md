---
title: Typed-nil 陷阱
tags: [concept, gotcha, nil, interface]
---

# Typed-nil 陷阱

## 现象

```go
type MyError struct{}
func (e *MyError) Error() string { return "my error" }

func mayFail() error {
    var p *MyError // nil pointer
    return p       // ← 装箱为 error 接口
}

if err := mayFail(); err != nil {
    fmt.Println(err.Error())  // panic: nil pointer dereference
}
```

`err != nil` 居然为 true！因为 `error` 接口由 `(type=*MyError, value=nil)` 组成，**类型非 nil**，所以接口非 nil。

## XKit 历史 Bug

### xsemaphore `doFallback` typed-nil 接口陷阱

```go
// 修复前
func doFallback(...) (Permit, error) {
    p := newNoopPermit(...)  // 可能返回 (*noopPermit)(nil), err
    return p, nil            // ← bug: p 是 typed-nil,但 Permit 接口非 nil
}

// 调用方
permit, _ := sem.Acquire(ctx)
if permit != nil {           // ← 误判为 true
    permit.Release(ctx)      // panic
}
```

修复（[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)）：
```go
p, err := newNoopPermit(...)
if err != nil {
    return nil, err          // ← 显式 nil 接口
}
return p, nil
```

### xtrace `OTelTracer` 零值满足接口

```go
var t OTelTracer       // 零值
var tracer Tracer = t  // ← 编译通过(L132 类型断言)
tracer.Inject(ctx, h)  // panic: propagator 是 nil
```

修复：Inject/Extract 添加 `t.propagator == nil` 守卫，降级为 no-op。

## 防御模式

### 1. 显式构造

```go
// 不要让 typed-nil 流出
p, err := newNoopPermit(...)
if err != nil {
    return nil, err  // ← Go 原生 nil
}
return p, nil
```

### 2. 接口入口零值守卫

```go
func (t OTelTracer) Inject(ctx, h) {
    if t.propagator == nil {  // ← 零值检查
        return  // no-op
    }
    t.propagator.Inject(ctx, h)
}
```

### 3. 测试用 Go 原生 `== nil` 不用 `testify`

```go
// 测试 typed-nil 修复时:
got, err := doFallback(...)
if got != nil { t.Fatal("want nil") }  // ✓ Go 原生

// 而非:
assert.Nil(t, got)  // ✗ testify 反射拆开对 typed-nil 也判 nil
```

## 相关

- 概念：[错误处理策略](02-error-handling-policy.md)
- 包：[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)、[xtrace](../04-packages/09-observability/05-xtrace.md)、[xmetrics](../04-packages/09-observability/02-xmetrics.md)
- 决策：[0001 构造函数返回 error](../01-decisions/0001-constructor-returns-error-not-panic.md)
