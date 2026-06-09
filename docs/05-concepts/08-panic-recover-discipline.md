---
title: Panic recover 纪律
tags: [concept, panic, recover, concurrency]
---

# Panic recover 纪律

## 何时 recover、何时不 recover

| 场景 | 决策 | 理由 |
|---|---|---|
| 包**内拥有** goroutine 的回调（xpool worker, xhealth statusListener, xelection observe, xconf onChange） | **必须 recover** | goroutine 是包的资产，panic 会让整个进程崩，必须隔离 |
| 业务方传入的 handler 在**业务方代码**触发 panic（xkafka/xpulsar `ConsumeLoop` 的 handler） | **刻意不 recover**（fail-fast） | handler 是业务代码，panic 表明业务 bug；自动 recover 掩盖问题 |
| 用户调用 `MarshalJSON` 等接口实现 | 入口包装 recover | 防御第三方代码污染调用栈 |

## XKit 实际案例

### Recover 的：
- [xpool.safeHandle](../04-packages/14-util/08-xpool.md)：worker 包装 handler，外层 recover 防止 worker 死亡
- [xhealth.statusListener](../04-packages/07-lifecycle/01-xhealth.md)：状态回调（FG-M fix 添加 recover）
- [xconf.safeCallback](../04-packages/02-config/01-xconf.md)：配置变更回调
- [xelection.observe](../04-packages/05-distributed/03-xelection.md)：watch 中断/session 过期回调
- [xjson.PrettyE](../04-packages/14-util/03-xjson.md)：拦截 MarshalJSON panic 转 ErrMarshal
- [xdbg.safeCallback](../04-packages/04-debug/01-xdbg.md)

### 不 recover 的：
- [mqcore.RunConsumeLoop](../04-packages/06-internal/02-mqcore.md) → 透传给 xkafka/xpulsar ConsumeLoop
- 业务方传给 xrun 的 Job

## 嵌套 recover：防 logger 二次崩

```go
func safeHandle(handler func(T), task T) {
    defer func() {
        if r := recover(); r != nil {
            // 外层 recover 防 handler panic
            defer func() {
                _ = recover()  // ← 内层 recover 防 logger 自身 panic
            }()
            logger.Error("handler panic", slog.Any("panic", r))
        }
    }()
    handler(task)
}
```

理由：handler panic 后 logger 也可能 panic（如 typed-nil handler），不能让 logger 把 worker 拖死。

## panic value 保留 error 链

如果 `recover()` 返的值实现 `error` 接口，用 `%w` 保留：

```go
defer func() {
    if r := recover(); r != nil {
        if e, ok := r.(error); ok {
            err = fmt.Errorf("xjson: %w: %w", ErrMarshal, e)
        } else {
            err = fmt.Errorf("xjson: %w: %v", ErrMarshal, r)
        }
    }
}()
```

（xjson 实例）

## 相关

- 概念：[错误处理策略](02-error-handling-policy.md)、[并发安全](01-concurrency-safety.md)
- 模式：[Fail-Fast 消费循环](../06-patterns/05-fail-fast-consume-loop.md)
- 包：[xpool](../04-packages/14-util/08-xpool.md)、[xhealth](../04-packages/07-lifecycle/01-xhealth.md)、[xjson](../04-packages/14-util/03-xjson.md)、[mqcore](../04-packages/06-internal/02-mqcore.md)
