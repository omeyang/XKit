// Package xcron 提供分布式定时任务调度能力。
//
// # 概述
//
// xcron 基于 [robfig/cron/v3] 构建，增加分布式锁支持，
// 解决多副本部署时定时任务重复执行的问题。
//
// # 核心概念
//
//   - Scheduler: 调度器，负责管理和执行定时任务
//   - Job: 任务接口，定义任务执行逻辑
//   - Locker: 分布式锁接口，确保多副本场景下任务只执行一次
//
// # 部署场景
//
//   - 单副本：使用 NoopLocker，无锁直接执行
//   - 多副本（在线）：使用 RedisLocker，基于 Redis 分布式锁
//   - 多副本（离线）：使用 K8sLocker，基于 K8S Lease 资源
//
// # 快速开始
//
//	// 单副本
//	scheduler := xcron.New()
//	scheduler.AddFunc("@every 1m", func(ctx context.Context) error {
//	    return doSomething(ctx)
//	}, xcron.WithName("my-task"))
//	scheduler.Start()
//	defer scheduler.Stop()
//
//	// 多副本（Redis 锁）
//	locker := xcron.NewRedisLocker(redisClient)
//	scheduler := xcron.New(xcron.WithLocker(locker))
//
// # 任务选项
//
//   - WithName: 任务名（用作锁 key，必须唯一）
//   - WithJobLocker: 任务级分布式锁
//   - WithLockTTL: 锁超时时间（默认 5 分钟）
//   - WithTimeout: 任务执行超时
//   - WithRetry: 重试策略
//   - WithImmediate: 注册后立即执行一次
//
// # 任务实现要求
//
// 任务函数必须正确响应 context 取消信号。当锁续期失败或任务超时时，
// xcron 会通过取消 context 来中止任务。如果任务不检查 ctx.Done()，
// 可能在锁已失效后继续执行，导致并发问题。
//
// 推荐模式：
//
//	func myTask(ctx context.Context) error {
//	    for i := 0; i < 100; i++ {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err() // 响应取消
//	        default:
//	        }
//	        // 执行工作...
//	    }
//	    return nil
//	}
//
// # 分布式锁
//
// xcron 使用 LockHandle 模式，每次 TryLock 成功返回封装唯一 token 的 handle，
// 确保同一进程内多个 goroutine 获取同一锁不会互相干扰。
//
// 提供"尽力互斥"（best-effort mutual exclusion）语义。
// 如需强一致性互斥，请使用 xdlock 包的 etcd 后端。
//
// [robfig/cron/v3]: https://github.com/robfig/cron
package xcron
