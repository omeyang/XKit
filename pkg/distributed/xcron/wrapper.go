package xcron

import (
	"context"
	"log"
	"sync"
	"time"
)

// jobWrapper 包装原始任务，添加锁、超时、重试等能力。
// 实现 cron.Job 接口，以便被 robfig/cron 调度。
type jobWrapper struct {
	job    Job
	opts   *jobOptions
	locker Locker
	logger Logger
	stats  *Stats // 执行统计
}

// renewHandle 保存单次任务执行的锁续期状态
// 每次 Run() 执行独立创建，避免并发执行间的竞态
type renewHandle struct {
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	renewFailed chan struct{}      // 续期失败信号
	taskCancel  context.CancelFunc // 用于在续期失败时取消任务
	lockHandle  LockHandle         // 本次获取的锁句柄（用于 Unlock 和 Renew）
}

// newJobWrapper 创建任务包装器
func newJobWrapper(job Job, locker Locker, logger Logger, stats *Stats, opts *jobOptions) *jobWrapper {
	return &jobWrapper{
		job:    job,
		opts:   opts,
		locker: locker,
		logger: logger,
		stats:  stats,
	}
}

// Run 实现 cron.Job 接口
func (w *jobWrapper) Run() {
	ctx := context.Background()
	startTime := time.Now()

	// 创建可取消的任务上下文，用于续期失败时中止任务
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	// 1. 尝试获取锁（如果配置了任务名）
	rh := w.tryAcquireLock(taskCtx, taskCancel)
	if rh == nil && w.opts.name != "" && w.locker != nil {
		// 需要锁但未获取到，记录跳过
		if w.stats != nil {
			w.stats.recordSkip(w.opts.name)
		}
		return
	}

	// 2. 超时控制
	taskCtx, cancel := w.applyTimeout(taskCtx)
	if cancel != nil {
		defer cancel()
	}

	// 3. 链路追踪
	taskCtx, span := w.startSpan(taskCtx)
	if span != nil {
		defer span.End()
	}

	// 4. 执行钩子 BeforeJob（正序）
	taskCtx = w.runBeforeHooks(taskCtx)

	// 5. 执行任务（可能带重试）
	err := w.executeJob(taskCtx, rh)
	duration := time.Since(startTime)

	// 6. 执行钩子 AfterJob（逆序，类似 defer）
	w.runAfterHooks(taskCtx, duration, err)

	// 7. 记录统计
	if w.stats != nil {
		w.stats.recordExecution(w.opts.name, duration, err)
	}

	// 8. 记录日志结果
	w.logResult(taskCtx, span, err)
}

// tryAcquireLock 尝试获取分布式锁
// 返回 renewHandle 用于后续停止续期；如果不需要锁或获取失败返回 nil
// taskCancel 用于在续期失败时取消任务执行
func (w *jobWrapper) tryAcquireLock(ctx context.Context, taskCancel context.CancelFunc) *renewHandle {
	if w.opts.name == "" || w.locker == nil {
		return nil // 不需要锁
	}

	// 应用锁获取超时，防止底层存储响应慢导致 goroutine 长时间阻塞
	lockCtx := ctx
	if w.opts.lockTimeout > 0 {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, w.opts.lockTimeout)
		defer cancel()
	}

	handle, err := w.locker.TryLock(lockCtx, w.opts.name, w.opts.lockTTL)
	if err != nil {
		w.logWarn(ctx, "failed to acquire lock",
			"job", w.opts.name, "error", err)
		return nil
	}
	if handle == nil {
		w.logDebug(ctx, "lock not acquired, skipping",
			"job", w.opts.name)
		return nil
	}

	// 启动锁续期，返回 renewHandle（包含 LockHandle）
	return w.startRenew(ctx, taskCancel, handle)
}

// applyTimeout 应用超时控制
func (w *jobWrapper) applyTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.opts.timeout > 0 {
		return context.WithTimeout(ctx, w.opts.timeout)
	}
	return ctx, nil
}

// startSpan 启动链路追踪
func (w *jobWrapper) startSpan(ctx context.Context) (context.Context, Span) {
	if w.opts.tracer != nil {
		return w.opts.tracer.Start(ctx, "xcron."+w.opts.name)
	}
	return ctx, nil
}

// executeJob 执行任务（可能带重试）
func (w *jobWrapper) executeJob(ctx context.Context, rh *renewHandle) error {
	// 确保释放锁
	if rh != nil && rh.lockHandle != nil {
		defer func() {
			w.stopRenew(rh)
			// 使用独立的 context 进行 Unlock，避免任务取消导致释放失败
			unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer unlockCancel()
			if err := rh.lockHandle.Unlock(unlockCtx); err != nil {
				w.logWarn(ctx, "failed to release lock",
					"job", w.opts.name, "error", err)
			}
		}()
	}

	if w.opts.retry != nil {
		return w.runWithRetry(ctx)
	}
	return w.job.Run(ctx)
}

// logResult 记录任务执行结果
func (w *jobWrapper) logResult(ctx context.Context, span Span, err error) {
	if err != nil {
		w.logError(ctx, "job failed",
			"job", w.opts.name, "error", err)
		if span != nil {
			span.RecordError(err)
		}
	} else {
		w.logDebug(ctx, "job completed",
			"job", w.opts.name)
	}
}

// runWithRetry 带重试执行任务
func (w *jobWrapper) runWithRetry(ctx context.Context) error {
	for attempt := 1; ; attempt++ {
		err := w.job.Run(ctx)
		if err == nil {
			return nil // 成功
		}

		// 检查是否应该重试
		if !w.opts.retry.ShouldRetry(attempt, err) {
			return err
		}

		// 计算退避时间
		var backoff time.Duration
		if w.opts.backoff != nil {
			backoff = w.opts.backoff.NextDelay(attempt)
		}

		w.logWarn(ctx, "job failed, will retry",
			"job", w.opts.name, "attempt", attempt, "backoff", backoff, "error", err)

		// 等待退避时间
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
}

// startRenew 启动锁续期协程，返回用于停止的 handle
// taskCancel 用于在续期失败时取消任务执行，防止锁过期后继续执行导致并发
// lockHandle 是本次获取的锁句柄，用于续期和释放
func (w *jobWrapper) startRenew(ctx context.Context, taskCancel context.CancelFunc, lockHandle LockHandle) *renewHandle {
	if lockHandle == nil {
		return nil
	}

	// 续期间隔为 TTL 的 1/3
	interval := w.opts.lockTTL / 3
	if interval < time.Second {
		interval = time.Second
	}

	renewCtx, cancel := context.WithCancel(ctx)
	rh := &renewHandle{
		cancel:      cancel,
		renewFailed: make(chan struct{}),
		taskCancel:  taskCancel,
		lockHandle:  lockHandle,
	}
	rh.wg.Add(1)

	go func() {
		defer rh.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				if err := lockHandle.Renew(renewCtx, w.opts.lockTTL); err != nil {
					w.logError(ctx, "lock renewal failed, canceling task to prevent concurrent execution",
						"job", w.opts.name, "error", err)
					// 续期失败时取消任务执行，防止锁过期后并发执行
					if taskCancel != nil {
						taskCancel()
					}
					close(rh.renewFailed)
					return
				}
			}
		}
	}()

	return rh
}

// stopRenew 停止锁续期
func (w *jobWrapper) stopRenew(rh *renewHandle) {
	if rh == nil || rh.cancel == nil {
		return
	}
	rh.cancel()
	rh.wg.Wait()
}

// 日志辅助方法

func (w *jobWrapper) logDebug(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Debug(ctx, msg, args...)
	}
}

func (w *jobWrapper) logWarn(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Warn(ctx, msg, args...)
	} else {
		log.Printf("[WARN] xcron: %s %v", msg, args)
	}
}

func (w *jobWrapper) logError(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Error(ctx, msg, args...)
	} else {
		log.Printf("[ERROR] xcron: %s %v", msg, args)
	}
}

// runBeforeHooks 执行 BeforeJob 钩子（正序）
func (w *jobWrapper) runBeforeHooks(ctx context.Context) context.Context {
	if len(w.opts.hooks) == 0 {
		return ctx
	}

	for _, hook := range w.opts.hooks {
		ctx = hook.BeforeJob(ctx, w.opts.name)
	}
	return ctx
}

// runAfterHooks 执行 AfterJob 钩子（逆序，类似 defer）
func (w *jobWrapper) runAfterHooks(ctx context.Context, duration time.Duration, err error) {
	if len(w.opts.hooks) == 0 {
		return
	}

	// 逆序执行，类似 defer 的行为
	for i := len(w.opts.hooks) - 1; i >= 0; i-- {
		w.opts.hooks[i].AfterJob(ctx, w.opts.name, duration, err)
	}
}
