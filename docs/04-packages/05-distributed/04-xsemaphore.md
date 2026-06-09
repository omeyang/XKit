---
package: pkg/distributed/xsemaphore
stability: Beta
coverage: 94.1%
tags: [distributed, semaphore, redis, rate-limit]
related:
  - ../../05-concepts/02-error-handling-policy.md
  - ../../05-concepts/10-typed-nil-trap.md
  - ../../06-patterns/04-distributed-lock.md
---

# pkg/distributed/xsemaphore

Redis 分布式信号量。支持容量限制、租户配额、Lua 脚本原子操作 + Fallback 降级、多副本协调、自动续期。**经多轮对抗审查稳定。**

## 用途

- 限制并发数：如"全集群同时只能 N 个 worker 处理某资源"
- 多租户配额：每个 tenant 独立配额，避免单租户耗尽全局容量
- 防雪崩：Redis 失联时可选 Fallback 到本地信号量（FailOpen / FailClosed）

## 不适用场景

- 强一致互斥（用 [xdlock](02-xdlock.md) 分布式锁）
- 仅单进程内：用 `golang.org/x/sync/semaphore`，xsemaphore 强依赖 Redis

## 快速上手

```go
import (
    "context"
    "time"

    "github.com/omeyang/xkit/pkg/distributed/xsemaphore"
    "github.com/redis/go-redis/v9"
)

// 1. 预热 Lua 脚本（启动时调用一次）
ctx := context.Background()
_ = xsemaphore.WarmupScripts(ctx, redisClient)

// 2. 创建信号量
sem, err := xsemaphore.New(redisClient, "my-resource",
    xsemaphore.WithFallback(xsemaphore.FallbackOpen),
    xsemaphore.WithPodCount(3),
)
if err != nil { /* ... */ }

// 3. 获取许可（带容量、TTL、租户）
permit, err := sem.Acquire(ctx,
    xsemaphore.WithCapacity(10),
    xsemaphore.WithTTL(30*time.Second),
    xsemaphore.WithTenantID("tenant-A"),
    xsemaphore.WithTenantQuota(3),
    xsemaphore.WithMaxRetries(5),
    xsemaphore.WithRetryDelay(100*time.Millisecond),
)
if err != nil {
    if xsemaphore.IsCapacityFull(err) { /* 排队或降级 */ }
    return err
}
defer permit.Release(ctx)

// 4. 长任务可续期
permit.Extend(ctx, 30*time.Second)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Semaphore` | 信号量接口（`Acquire/Release/Extend/Query`）|
| `Permit` | 许可句柄（`Release/Extend/ID/Resource/Capacity`）|
| `FallbackStrategy` | `FallbackOpen` / `FallbackClosed` / 无（默认）|
| `WarmupScripts(ctx, client)` | 启动时预热 Lua 脚本到 Redis SHA |
| `WithFallback / WithPodCount / WithOnFallback` | 降级策略 |
| `WithMaxRetries / WithRetryDelay` | 重试控制（上限 `MaxMaxRetries=10000`）|
| `WithIDGenerator` | 自定义 ID 生成器（默认 xid）|
| `WithScriptMode(rediscompat.ScriptMode)` | 代理兼容（Predixy 等不支持 EVAL）|
| `IsRedisError / IsCapacityFull / IsTenantQuotaExceeded / IsPermitNotHeld / IsRetryable` | 错误分类断言 |
| `ClassifyError(err) string` | 标准化分类标签（用于 metrics）|
| `NewMetrics(meterProvider, opts...)` | OTel 指标采集 |
| `Attr*` 系列 | `slog.Attr` 工厂（PermitID/Resource/TenantID/Capacity 等）|

## 设计要点

- **Lua 脚本原子性**：`acquire.lua` / `release.lua` 在 Redis 单线程下保证原子
- **Hash tag**：所有 key 用 `{resource}:` 前缀做 hash 分槽，集群兼容
- **资源名验证**：`{}:` 与空白字符禁止用于 resource/tenantID/keyPrefix
- **typed-nil 陷阱已修**：`doFallback` 显式 `(nil, err)` 返回，避免 `(*noopPermit)(nil)` 被装箱为非 nil 接口（见 [typed-nil 陷阱](../../05-concepts/10-typed-nil-trap.md)）
- **Close 幂等**：`atomic.Swap` 保证多次 Close 安全
- **`%v` 抽象边界**：`ErrIDGenerationFailed` 用 `%v` 不 `%w`（设计决策，见 [错误处理策略](../../05-concepts/02-error-handling-policy.md)）
- **背景清理 goroutine**：`localSemaphore` 用 `cleanupWg.Wait()` 保证 Close 时退出
- **指标前缀**：`xsemaphore.*`（匹配 Meter scope）

## Mock 子包

`pkg/distributed/xsemaphore/xsemaphoremock`：mockgen 生成的 gomock 桩，供下游单测使用。详见 [testkit MOC](../13-testkit/00-index.md)。

## 相关

- 概念：
  - [错误处理策略](../../05-concepts/02-error-handling-policy.md)
  - [Typed-nil 陷阱](../../05-concepts/10-typed-nil-trap.md)
  - [Nil context 防御](../../05-concepts/06-nil-context-defense.md)
- 模式：
  - [分布式锁](../../06-patterns/04-distributed-lock.md)
- ADR：
  - [0001 构造函数返回 error](../../01-decisions/0001-constructor-returns-error-not-panic.md)
  - [0004 错误包装策略](../../01-decisions/0004-error-wrapping-policy.md)
- API：[api.md#pkgdistributedxsemaphore](../../03-conventions/01-api.md#pkgdistributedxsemaphore)
