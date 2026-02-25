package xcron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// jobWrapper 包装原始任务，添加锁、超时、重试等能力。
// 实现 cron.Job 接口，以便被 robfig/cron 调度。
type jobWrapper struct {
	job     Job
	opts    *jobOptions
	locker  Locker
	logger  Logger
	stats   *Stats          // 执行统计
	baseCtx context.Context // 可选: 立即执行任务使用的可取消上下文
}

// renewHandle 保存单次任务执行的锁续期状态
// 每次 Run() 执行独立创建，避免并发执行间的竞态
type renewHandle struct {
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	taskCancel context.CancelFunc // 用于在续期失败时取消任务
	lockHandle LockHandle         // 本次获取的锁句柄（用于 Unlock 和 Renew）
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
	if w.baseCtx != nil {
		ctx = w.baseCtx
	}
	startTime := time.Now()

	// 创建可取消的任务上下文，用于续期失败时中止任务
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	// 1. 尝试获取锁（如果配置了任务名）
	rh, lockErr := w.tryAcquireLock(taskCtx, taskCancel)
	if rh == nil && w.opts.name != "" && w.locker != nil {
		// 需要锁但未获取到
		if w.stats != nil {
			if lockErr != nil {
				// 锁服务异常，计入失败（而非跳过），便于健康检查发现问题
				w.stats.recordExecution(w.opts.name, 0, lockErr)
			} else {
				// 锁竞争失败（正常跳过）
				w.stats.recordSkip(w.opts.name)
			}
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

	// 4. 执行钩子 BeforeJob（正序），每个钩子独立 panic 保护
	taskCtx = w.runBeforeHooks(taskCtx)

	// 5. 执行任务（可能带重试）
	err := w.executeJob(taskCtx, rh)
	duration := time.Since(startTime)

	// 6. 执行钩子 AfterJob（逆序，类似 defer），每个钩子独立 panic 保护
	w.runAfterHooks(taskCtx, duration, err)

	// 7. 记录统计
	if w.stats != nil {
		w.stats.recordExecution(w.opts.name, duration, err)
	}

	// 8. 记录日志结果
	w.logResult(taskCtx, span, err)
}

// tryAcquireLock 尝试获取分布式锁。
//
// 返回值:
//   - renewHandle: 成功时返回续期句柄，不需要锁或获取失败返回 nil
//   - error: 锁服务异常时返回错误（区别于锁竞争失败的 nil,nil）
//
// taskCancel 用于在续期失败时取消任务执行
func (w *jobWrapper) tryAcquireLock(ctx context.Context, taskCancel context.CancelFunc) (*renewHandle, error) {
	if w.opts.name == "" || w.locker == nil {
		return nil, nil // 不需要锁
	}

	// 应用锁获取超时，防止底层存储响应慢导致 goroutine 长时间阻塞
	lockCtx := ctx
	if w.opts.lockTimeout > 0 {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, w.opts.lockTimeout)
		defer cancel()
	}

	handle, err := w.safeTryLock(lockCtx)
	if err != nil {
		w.logWarn(ctx, "lock service error",
			"job", w.opts.name, "error", err)
		return nil, err // 锁服务异常
	}
	if handle == nil {
		w.logDebug(ctx, "lock not acquired, skipping",
			"job", w.opts.name)
		return nil, nil // 锁竞争失败（正常）
	}

	// 启动锁续期，返回 renewHandle（包含 LockHandle）
	return w.startRenew(ctx, taskCancel, handle), nil
}

// safeTryLock 安全调用 TryLock，将 panic 转为 error。
// 设计决策: 防止 Locker 实现（第三方/基础设施）panic 导致调度器 goroutine 崩溃。
func (w *jobWrapper) safeTryLock(ctx context.Context) (handle LockHandle, err error) {
	defer func() {
		if r := recover(); r != nil {
			handle = nil
			err = fmt.Errorf("xcron: locker.TryLock panicked: %v", r)
		}
	}()
	return w.locker.TryLock(ctx, w.opts.name, w.opts.lockTTL)
}

// applyTimeout 应用超时控制
func (w *jobWrapper) applyTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.opts.timeout > 0 {
		return context.WithTimeout(ctx, w.opts.timeout)
	}
	return ctx, nil
}

// startSpan 启动链路追踪。
// 设计决策: 独立 panic 隔离，防止 tracer 实现 panic 导致跳过 executeJob（从而跳过 Unlock），
// 锁虽有 TTL 兜底，但显式释放可避免不必要的等待。
func (w *jobWrapper) startSpan(ctx context.Context) (resultCtx context.Context, resultSpan Span) {
	if w.opts.tracer == nil {
		return ctx, nil
	}
	defer func() {
		if r := recover(); r != nil {
			w.logError(ctx, "tracer.Start panicked",
				"job", w.opts.name, "panic", r)
			resultCtx = ctx
			resultSpan = nil
		}
	}()
	return w.opts.tracer.Start(ctx, "xcron."+w.opts.name)
}

// executeJob 执行任务（可能带重试），包含 panic 恢复
func (w *jobWrapper) executeJob(ctx context.Context, rh *renewHandle) (err error) {
	// 确保释放锁
	if rh != nil && rh.lockHandle != nil {
		defer func() {
			w.stopRenew(rh)
			// 使用独立的 context 进行 Unlock，避免任务取消导致释放失败
			unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer unlockCancel()
			if unlockErr := rh.lockHandle.Unlock(unlockCtx); unlockErr != nil {
				w.logWarn(ctx, "failed to release lock",
					"job", w.opts.name, "error", unlockErr)
			}
		}()
	}

	// panic 恢复：防止单个任务 panic 导致整个调度器崩溃
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("xcron: job %q panicked: %v", w.opts.name, r)
		}
	}()

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

// runWithRetry 带重试执行任务。
// 每次重试独立 recover，将 panic 转为 error 参与重试判断，
// 避免 panic 中断整个重试循环且掩盖之前的重试错误。
func (w *jobWrapper) runWithRetry(ctx context.Context) error {
	for attempt := 1; ; attempt++ {
		err := w.safeRunJob(ctx)
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
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

// safeRunJob 执行一次任务，将 panic 转为 error。
// 用于 runWithRetry 中每次重试的独立 panic 保护。
func (w *jobWrapper) safeRunJob(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("xcron: job %q panicked: %v", w.opts.name, r)
		}
	}()
	return w.job.Run(ctx)
}

// startRenew 启动锁续期协程，返回用于停止的 handle
// taskCancel 用于在续期失败时取消任务执行，防止锁过期后继续执行导致并发
// lockHandle 是本次获取的锁句柄，用于续期和释放
func (w *jobWrapper) startRenew(ctx context.Context, taskCancel context.CancelFunc, lockHandle LockHandle) *renewHandle {
	if lockHandle == nil {
		return nil
	}

	// 续期间隔为 TTL 的 1/3
	interval := max(w.opts.lockTTL/3, time.Second)

	renewCtx, cancel := context.WithCancel(ctx)
	rh := &renewHandle{
		cancel:     cancel,
		taskCancel: taskCancel,
		lockHandle: lockHandle,
	}
	rh.wg.Add(1)
	go func() {
		defer rh.wg.Done()
		// 设计决策: panic 隔离防止 lockHandle.Renew 实现 panic 导致进程崩溃，
		// 续期协程 panic 后取消任务，与续期失败行为一致。
		defer func() {
			if r := recover(); r != nil {
				w.logError(ctx, "lock renewal panicked, canceling task",
					"job", w.opts.name, "panic", r)
				if taskCancel != nil {
					taskCancel()
				}
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				// 设计决策: 每次续期使用独立超时（取 lockTimeout 和 lockTTL/3 的较小值），
				// 防止后端响应慢导致续期协程卡住，进而导致 stopRenew 的 wg.Wait() 阻塞。
				renewTimeout := min(w.opts.lockTimeout, w.opts.lockTTL/3)
				if renewTimeout <= 0 {
					renewTimeout = 5 * time.Second
				}
				callCtx, callCancel := context.WithTimeout(renewCtx, renewTimeout)
				err := lockHandle.Renew(callCtx, w.opts.lockTTL)
				callCancel()
				if err != nil {
					w.logError(ctx, "lock renewal failed, canceling task to prevent concurrent execution",
						"job", w.opts.name, "error", err)
					// 续期失败时取消任务执行，防止锁过期后并发执行
					if taskCancel != nil {
						taskCancel()
					}
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

// logDebug 记录调试日志。
// 设计决策: 无 logger 时静默丢弃（不回退到 slog），因为 Debug 日志通常量大，
// 输出到默认 logger 会造成噪音。logWarn/logError 回退到 slog 是因为警告和错误不应被静默忽略。
func (w *jobWrapper) logDebug(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Debug(ctx, msg, args...)
	}
}

func (w *jobWrapper) logWarn(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Warn(ctx, msg, args...)
	} else {
		slog.WarnContext(ctx, "xcron: "+msg, args...)
	}
}

func (w *jobWrapper) logError(ctx context.Context, msg string, args ...any) {
	if w.logger != nil {
		w.logger.Error(ctx, msg, args...)
	} else {
		slog.ErrorContext(ctx, "xcron: "+msg, args...)
	}
}

// runBeforeHooks 执行 BeforeJob 钩子（正序）。
// 每个钩子独立 recover，防止单个钩子 panic 导致调度器崩溃。
func (w *jobWrapper) runBeforeHooks(ctx context.Context) context.Context {
	if len(w.opts.hooks) == 0 {
		return ctx
	}

	for _, hook := range w.opts.hooks {
		ctx = w.safeBeforeHook(ctx, hook)
	}
	return ctx
}

// safeBeforeHook 安全执行单个 BeforeJob 钩子，捕获 panic。
func (w *jobWrapper) safeBeforeHook(ctx context.Context, hook Hook) (result context.Context) {
	result = ctx
	defer func() {
		if r := recover(); r != nil {
			w.logError(ctx, "BeforeJob hook panicked",
				"job", w.opts.name, "panic", r)
			result = ctx // panic 时返回原始 ctx
		}
	}()
	return hook.BeforeJob(ctx, w.opts.name)
}

// runAfterHooks 执行 AfterJob 钩子（逆序，类似 defer）。
// 每个钩子独立 recover，防止单个钩子 panic 导致调度器崩溃。
func (w *jobWrapper) runAfterHooks(ctx context.Context, duration time.Duration, err error) {
	if len(w.opts.hooks) == 0 {
		return
	}

	// 逆序执行，类似 defer 的行为
	for i := len(w.opts.hooks) - 1; i >= 0; i-- {
		w.safeAfterHook(ctx, w.opts.hooks[i], duration, err)
	}
}

// safeAfterHook 安全执行单个 AfterJob 钩子，捕获 panic。
func (w *jobWrapper) safeAfterHook(ctx context.Context, hook Hook, duration time.Duration, err error) {
	defer func() {
		if r := recover(); r != nil {
			w.logError(ctx, "AfterJob hook panicked",
				"job", w.opts.name, "panic", r)
		}
	}()
	hook.AfterJob(ctx, w.opts.name, duration, err)
}
