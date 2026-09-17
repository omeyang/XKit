---
title: 并发安全
tags: [concept, concurrency, goroutine]
---

# 并发安全

## goroutine 生命周期

XKit 包内 goroutine 必须有**明确的退出协议**。常见模式：

### 1. ctx-bound

```go
func (s *Service) Start(ctx context.Context) {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case ev := <-s.events:
                s.handle(ev)
            }
        }
    }()
}
```

ctx 是唯一退出信号。

### 2. done channel + WaitGroup

```go
type Service struct {
    done chan struct{}
    wg   sync.WaitGroup
}

func (s *Service) Start() {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        for {
            select {
            case <-s.done:
                return
            case ev := <-s.events:
                s.handle(ev)
            }
        }
    }()
}

func (s *Service) Close() error {
    close(s.done)
    s.wg.Wait()   // ← 保证退出
    return nil
}
```

XKit 中 [xsemaphore.localSemaphore](../04-packages/05-distributed/04-xsemaphore.md) 用此模式：`cleanupWg.Wait()` 保证 `backgroundCleanupLoop` 退出。

### 3. closeOnce 幂等关闭

```go
type Service struct {
    closeOnce sync.Once
    closed    atomic.Bool
}

func (s *Service) Close() error {
    var err error
    s.closeOnce.Do(func() {
        s.closed.Store(true)
        // 清理
    })
    return err
}
```

避免重复 Close 二次释放资源。

## 共享资源保护

### sync.Mutex 范式

```go
type cache struct {
    mu sync.RWMutex
    m  map[string]V
}

func (c *cache) Get(k string) (V, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    v, ok := c.m[k]
    return v, ok
}
```

读多写少用 `RWMutex`，避免 writer 锁等待 reader 长时间持锁。

### atomic.Bool 状态标记

```go
type pool struct {
    closed atomic.Bool
}

func (p *pool) Submit(t T) error {
    if p.closed.Load() {  // ← 无锁快路径
        return ErrPoolStopped
    }
    // ...
}
```

## TOCTOU 窗口

`if !closed { do() }` 与 `Close()` 之间有窗口期：

```go
if !p.closed.Load() {  // ← Check
    // 此时其他 goroutine 调用 Close, 设 closed=true
    p.queue <- task    // ← Operation (Time of use)
    // 但 queue 可能已 close → panic
}
```

XKit 中的解法：
- [xpool](../04-packages/14-util/08-xpool.md) `submitMu RWMutex` 把 check 和 send 包在 RLock 内
- [xlru](../04-packages/14-util/05-xlru.md) Close TOCTOU 窗口**文档化为可接受**（残留 entry 不可见且 GC）

## map 并发访问

Go map 不保证并发安全。同时读写会 fatal error（不可 recover）。规则：
- 任何并发场景的 map 用 `sync.Map` 或 `sync.RWMutex` 包装
- 例外：分片 map（如 [xkeylock](../04-packages/14-util/04-xkeylock.md)）每 shard 内串行化

## channel 关闭模式

```go
// 标准 Go pattern: close + select
close(done)

for {
    select {
    case <-done:
        return  // ← close 后立即返回
    case x := <-other:
        process(x)
    }
}
```

不要重复 close（panic: close of closed channel）：用 `sync.Once`。

## goleak 测试

XKit 用 `go.uber.org/goleak` 在 TestMain 检测 goroutine 泄漏：

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

## 相关

- 概念：[Panic recover 纪律](08-panic-recover-discipline.md)
- 包：[xpool](../04-packages/14-util/08-xpool.md)、[xkeylock](../04-packages/14-util/04-xkeylock.md)、[xlru](../04-packages/14-util/05-xlru.md)、[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)
