---
package: pkg/distributed/xcron
stability: Beta
coverage: 94.1%
tags: [distributed, cron, scheduler]
---

# pkg/distributed/xcron

分布式定时任务。基于 `robfig/cron`，加上锁层（Redis / etcd / Noop）保证多副本同一时刻只有一个执行。

## 用途

- 定时任务（如每天 00:00 清理过期数据）
- 多副本环境去重（避免每个 pod 都跑一次）
- 任务级钩子（before/after/重试/超时/链路追踪）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/distributed/xcron"

s := xcron.New(
    xcron.WithLocker(xcron.NewRedisLocker(redisClient)),
    xcron.WithSeconds(),
)
defer s.Stop()

id, err := s.AddFunc("0 */5 * * * *", func(ctx context.Context) error {
    return doWork(ctx)
}, xcron.WithName("sync-job"), xcron.WithTimeout(2*time.Minute))

s.Start()
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Scheduler` interface | `AddFunc / AddJob / Remove / Start / Stop / Stats` |
| `Job` interface | `Run(ctx) error` |
| `Locker` interface | `TryLock(ctx, key, ttl) (LockHandle, error)` |
| `NewRedisLocker / NoopLocker` | 锁后端 |
| `Hook` interface | `BeforeJob / AfterJob` |
| `WithSeconds / WithLocker / WithLogger / WithLocation` | 调度器选项 |
| `WithName / WithJobLocker / WithLockTTL / WithTimeout / WithRetry / WithBackoff / WithTracer / WithImmediate / WithHook` | 任务选项 |

## 设计要点

- **锁键 = 任务名**：`WithName` 必须设，否则无法去重
- **`NoopLocker`** 用于单副本环境
- **`WithImmediate`** 注册后立即执行一次（不等下次调度时刻）
- **任务超时**：`WithTimeout` 通过 ctx cancel 传播

## 相关

- 模式：
  - [Distributed Lock](../../06-patterns/04-distributed-lock.md)
- API：[api.md#pkgdistributedxcron](../../03-conventions/01-api.md#pkgdistributedxcron)
