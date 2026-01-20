# 模块审查：pkg/distributed（分布式协调）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/distributed/
├── xcron/    # 分布式定时任务，基于 robfig/cron/v3
└── xdlock/   # 分布式锁，支持 etcd 和 Redis (Redlock)
```

设计理念：
- 工厂函数：NewEtcdFactory, NewRedisFactory
- 底层暴露：Session()/Redsync()/Mutex()/Cron()
- 增值功能：健康检查、统一错误处理、自动续期

包间关系：
```
xdlock (分布式锁)
    ↓ 可被使用（通过 WithExternalLock）
xcache (缓存)

xcron (定时任务)
    ├─ 使用 xcron.Locker 接口（NoopLocker/RedisLocker/K8sLocker）
    └─ 集成 xretry, xmetrics, xlog, xctx
```

---

## xdlock：分布式锁

**职责**：提供分布式锁的统一封装，支持 etcd 和 Redis 后端。

**关键文件**：
- `etcd.go` - etcd 后端实现
- `redis.go` - Redis (redsync) 后端实现
- `options.go` - 配置选项
- `errors.go` - 错误定义

**核心接口**：
```go
type Factory interface {
    NewMutex(key string, opts ...MutexOption) Locker
    Health(ctx context.Context) error
    Close() error
}

type Locker interface {
    Lock(ctx context.Context) error
    TryLock(ctx context.Context) error
    Unlock(ctx context.Context) error
    Extend(ctx context.Context) error
}
```

**后端差异**：
| 特性 | etcd | Redis (redsync) |
|------|------|-----------------|
| 续期方式 | 自动（Session） | 手动（Extend） |
| Extend() | 返回 ErrExtendNotSupported | 正常工作 |
| 多节点支持 | 原生（etcd 集群） | Redlock 算法 |
| 锁释放 | 立即生效 | 立即生效 |

**etcd 后端**：
```go
// 使用 xetcd.Config
factory, client, err := xdlock.NewEtcdFactoryFromConfig(config, nil)
defer factory.Close()

locker := factory.NewMutex("my-resource")
if err := locker.Lock(ctx); err != nil {
    return err
}
defer locker.Unlock(ctx)
```

**Redis 后端**：
```go
// 单节点模式
factory, err := xdlock.NewRedisFactory(client)

// Redlock 多节点模式（推荐生产环境）
factory, err := xdlock.NewRedisFactory(client1, client2, client3)

locker := factory.NewMutex("my-resource", xdlock.WithExpiry(30*time.Second))
```

**与 xcache 的关系**：
| 场景 | 推荐方案 |
|------|----------|
| 简单缓存防击穿 | xcache 内置锁（默认） |
| 高可用缓存防击穿 | xcache + xdlock（通过 WithExternalLock） |
| 业务互斥（单节点） | xdlock.NewRedisFactory(client) |
| 业务互斥（多节点） | xdlock.NewRedisFactory(client1, client2, client3) |
| 强一致性互斥 | xdlock.NewEtcdFactory(etcdClient) |

---

## xcron：分布式定时任务

**职责**：基于 robfig/cron/v3 提供分布式锁支持的定时任务调度。

**关键文件**：
- `scheduler.go` - Scheduler 调度器
- `locker.go` - Locker 接口定义
- `locker_redis.go` - RedisLocker 实现
- `locker_k8s.go` - K8sLocker 实现
- `locker_noop.go` - NoopLocker 实现
- `stats.go` - 统计信息

**部署场景**：
| Locker | 适用场景 | 底层实现 |
|--------|---------|---------|
| NoopLocker | 单副本 | 无锁直接执行 |
| RedisLocker | 多副本（在线） | Redis SET NX |
| K8sLocker | 多副本（离线） | K8s Lease 资源 |

**快速开始**：
```go
// 单副本场景
scheduler := xcron.New()
scheduler.AddFunc("@every 1m", func(ctx context.Context) error {
    return doSomething(ctx)
}, xcron.WithName("my-task"))
scheduler.Start()
defer scheduler.Stop()

// 多副本场景（Redis 锁）
locker := xcron.NewRedisLocker(redisClient)
scheduler := xcron.New(xcron.WithLocker(locker))
scheduler.AddFunc("0 2 * * *", dailyReport, xcron.WithName("daily-report"))
scheduler.Start()

// 多副本场景（K8s Lease 锁）
locker, _ := xcron.NewK8sLocker(xcron.K8sLockerOptions{
    Namespace: "my-namespace",
    Identity:  os.Getenv("POD_NAME"),
})
scheduler := xcron.New(xcron.WithLocker(locker))
```

**LockHandle 模式**：
```go
type Locker interface {
    TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error)
}

type LockHandle interface {
    Unlock(ctx context.Context) error
    Renew(ctx context.Context, ttl time.Duration) error
    Key() string
}
```

每个 LockHandle 内部封装唯一 token（格式：identity:uuid），确保：
- 同一进程内多个 goroutine 获取同一锁不会互相干扰
- 一个 handle 的 Unlock 不会误删另一个 handle 的锁
- 续期操作只影响自己的锁

**任务选项**：
- `WithName`: 任务名（用作锁 key，必须唯一）
- `WithJobLocker`: 任务级分布式锁（覆盖全局）
- `WithLockTTL`: 锁超时时间（默认 5 分钟，最小 3 秒）
- `WithLockTimeout`: 锁获取超时
- `WithTimeout`: 任务执行超时
- `WithRetry`: 重试策略（集成 xretry）
- `WithBackoff`: 退避策略
- `WithTracer`: 链路追踪（集成 xmetrics）
- `WithImmediate`: 注册后立即执行一次

**锁语义**：

xcron 提供"尽力互斥"（best-effort mutual exclusion）：

正常保证：
- 同一时刻只有一个实例执行任务
- 自动续期防止长任务时锁过期
- 续期失败时自动取消任务

已知局限（适用于所有 TTL 分布式锁）：
- 无 fencing token：网络分区恢复后可能短暂并发
- 时钟偏移：K8sLocker 通过 ClockSkew 缓解
- GC 暂停：长 GC 可能导致锁过期

如需强一致性互斥，使用 xdlock 的 etcd 后端。

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**xdlock Redis 实现**：
- 是否正确使用 redsync 库？
- 单节点 vs 多节点的创建方式是否清晰？
- Extend() 是否使用 Lua 脚本验证 owner？
- Close() 是否正确清理资源？

**xdlock etcd 实现**：
- 是否正确使用 etcd/client/v3/concurrency？
- Session TTL 设置是否合理？
- Session 失效后的处理？
- Factory Close() 是否释放 Session？

**xcron LockHandle**：
- 每个 LockHandle 是否封装唯一 token？
- 同一进程内多个 goroutine 获取同一锁是否相互隔离？
- Unlock 是否验证 token 防止误删？
- Renew 是否只续期自己的锁？

**xcron Locker 实现**：
- RedisLocker 是否使用 Lua 脚本保证原子性？
- K8sLocker 是否正确使用 k8s.io/client-go/tools/leaderelection？
- K8sLocker 的 ClockSkew 容忍度是否合理？

**自动续期**：
- 续期间隔是否合理？（通常为 TTL 的 1/3 ~ 1/2）
- 续期 goroutine 的生命周期管理？
- 续期失败时是否取消任务执行？
- 续期 goroutine 是否有泄漏风险？

**资源生命周期**：
- Stop() 是否等待正在执行的任务？
- Stop() 后锁是否正确释放？
- 是否有 goroutine 泄漏？
- Factory.Close() 是否正确释放 Session/连接？

**故障场景**：
- 锁过期但任务未完成时的行为？
- Unlock 时锁已被他人获取？
- K8s Pod 滚动更新时锁如何转移？
- 任务 panic 时锁是否释放？
- 网络分区恢复后的行为？

**与 xretry 集成**：
- 重试期间锁是否持有？
- 重试失败后锁的释放时机？
- 重试与任务超时的交互？

**与 xcache 集成**：
- WithExternalLock 返回的 Unlocker 是否正确？
- ctx 传递是否正确？
- 长时间加载时锁是否续期？

---

## 资源生命周期

**xdlock.Factory**：
```
NewEtcdFactory(cli) / NewRedisFactory(clients...)
    ↓
创建 Session / Redsync
    ↓
factory.NewMutex(key) → 创建 Locker
    ↓
locker.Lock() / TryLock()
    ↓
locker.Extend() (仅 Redis)
    ↓
locker.Unlock()
    ↓
factory.Close() → 释放 Session / 断开连接
```

**xcron.Scheduler**：
```
xcron.New(xcron.WithLocker(locker))
    ↓
scheduler.AddFunc(spec, fn, opts...) → 注册任务
    ↓
scheduler.Start() → 启动调度器 + 底层 cron
    ↓
任务触发 → TryLock() → 执行 → 自动续期 → Unlock()
    ↓
scheduler.Stop() → 停止调度器 + 等待任务完成
```

