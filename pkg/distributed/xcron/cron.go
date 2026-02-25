package xcron

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

// ErrNilJob 表示任务为 nil。
var ErrNilJob = errors.New("xcron: job cannot be nil")

// ErrMissingName 表示配置了分布式锁但未设置任务名。
// 设计决策: 选择 fail-fast 而非静默降级，因为无名任务跳过加锁会导致多副本重复执行，
// 可能引发重复扣款、重复下发等生产事故。调用方必须显式使用 WithName 或 WithJobLocker(NoopLocker())。
var ErrMissingName = errors.New("xcron: job with distributed locker must have a name, use WithName() to set one")

// ErrDuplicateJobName 表示任务名已被注册。
// 同一调度器内任务名必须唯一，因为任务名用作分布式锁 key 和统计维度 key。
// 重复名称会导致两个逻辑不同的任务共享同一把锁和统计数据，互相干扰。
var ErrDuplicateJobName = errors.New("xcron: duplicate job name, each job must have a unique name within the same scheduler")

// cronScheduler 基于 robfig/cron/v3 的调度器实现
type cronScheduler struct {
	cron   *cron.Cron
	opts   *schedulerOptions
	locker Locker
	logger Logger
	stats  *Stats // 执行统计

	mu       sync.Mutex       // 保护 jobNames 的并发读写
	jobNames map[string]JobID // 已注册的任务名 → EntryID，用于唯一性校验和 Remove 时释放

	immediateWg     sync.WaitGroup     // 追踪 WithImmediate 启动的立即执行任务
	immediateCtx    context.Context    // 立即执行任务的可取消上下文
	immediateCancel context.CancelFunc // 取消立即执行任务
}

// New 创建新的调度器。
//
// 不带参数时使用默认配置（NoopLocker，本地时区，分钟级精度）。
//
// 用法：
//
//	// 单副本场景
//	scheduler := xcron.New()
//
//	// 多副本场景（Redis 锁）
//	scheduler := xcron.New(xcron.WithLocker(xcron.NewRedisLocker(redisClient)))
//
//	// 自定义配置
//	scheduler := xcron.New(
//	    xcron.WithLocker(locker),
//	    xcron.WithLogger(logger),
//	    xcron.WithSeconds(), // 启用秒级精度
//	)
func New(opts ...SchedulerOption) Scheduler {
	options := defaultSchedulerOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 创建底层 cron 实例
	cronOpts := []cron.Option{
		cron.WithLocation(options.location),
		cron.WithParser(options.parser),
	}

	c := cron.New(cronOpts...)

	immediateCtx, immediateCancel := context.WithCancel(context.Background())

	return &cronScheduler{
		cron:            c,
		opts:            options,
		locker:          options.locker,
		logger:          options.logger,
		stats:           newStats(),
		jobNames:        make(map[string]JobID),
		immediateCtx:    immediateCtx,
		immediateCancel: immediateCancel,
	}
}

// AddFunc 添加函数任务
func (s *cronScheduler) AddFunc(spec string, cmd func(ctx context.Context) error, opts ...JobOption) (JobID, error) {
	if cmd == nil {
		return 0, ErrNilJob
	}
	return s.AddJob(spec, JobFunc(cmd), opts...)
}

// AddJob 添加 Job 接口任务
func (s *cronScheduler) AddJob(spec string, job Job, opts ...JobOption) (JobID, error) {
	if job == nil {
		return 0, ErrNilJob
	}

	// 合并任务选项
	jobOpts := defaultJobOptions()
	for _, opt := range opts {
		opt(jobOpts)
	}

	// 确定使用的锁
	locker := jobOpts.locker
	if locker == nil {
		locker = s.locker
	}

	// 持有 mu 保护 validateJobName → cron.AddJob → jobNames 写入的完整序列，
	// 防止并发 AddJob/Remove 导致 map 竞态（fatal: concurrent map writes）。
	s.mu.Lock()

	// 前置校验：任务名 + 唯一性
	if err := s.validateJobName(jobOpts, locker); err != nil {
		s.mu.Unlock()
		return 0, err
	}

	// 创建包装器
	wrapper := newJobWrapper(job, locker, s.logger, s.stats, jobOpts)

	// 添加到底层 cron
	id, err := s.cron.AddJob(spec, wrapper)
	if err != nil {
		s.mu.Unlock()
		return 0, fmt.Errorf("xcron: failed to add job: %w", err)
	}

	// 注册任务名
	if jobOpts.name != "" {
		s.jobNames[jobOpts.name] = id
	}

	s.mu.Unlock()

	// 立即执行一次（如果配置了 WithImmediate）
	// 使用 WaitGroup 追踪，确保 Stop() 时能等待完成
	// 创建独立的包装器副本，使用可取消的上下文，以便 Stop() 时能中止
	// 设计决策: 浅拷贝 *wrapper 共享 opts 指针，opts 在创建后为只读，无并发写入风险。
	if jobOpts.immediate {
		s.immediateWg.Add(1)
		go func() {
			defer s.immediateWg.Done()
			w := *wrapper
			w.baseCtx = s.immediateCtx
			w.Run()
		}()
	}

	return id, nil
}

// validateJobName 校验任务名：分布式锁场景必须设置任务名，且名称不可重复。
func (s *cronScheduler) validateJobName(jobOpts *jobOptions, locker Locker) error {
	// 设计决策: 配置了分布式锁但未设置任务名时 fail-fast 返回错误，
	// 而非静默降级跳过加锁。静默降级在多副本场景下会导致重复执行。
	if jobOpts.name == "" {
		if _, isNoop := locker.(noopIndicator); !isNoop {
			return ErrMissingName
		}
		return nil
	}
	// 任务名唯一性校验：同名任务共享锁 key 和统计维度，会导致互相干扰
	if _, exists := s.jobNames[jobOpts.name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateJobName, jobOpts.name)
	}
	return nil
}

// Remove 移除任务
func (s *cronScheduler) Remove(id JobID) {
	s.mu.Lock()
	// 释放任务名注册，允许同名任务重新添加
	for name, registeredID := range s.jobNames {
		if registeredID == id {
			delete(s.jobNames, name)
			break
		}
	}
	s.mu.Unlock()
	s.cron.Remove(id)
}

// Start 启动调度器
func (s *cronScheduler) Start() {
	s.cron.Start()
}

// Stop 优雅停止，立即返回。
// 返回的 context 在所有运行中的任务（含 WithImmediate 启动的立即执行任务）完成后 Done。
// 立即执行任务的上下文会被取消，使其能尽快结束。
//
// 设计决策: Stop() 立即返回 context 而不阻塞，调用方通过 ctx.Done() 或
// select + time.After 做超时控制，避免 Stop() 本身卡死。
func (s *cronScheduler) Stop() context.Context {
	// 取消所有立即执行任务的上下文
	s.immediateCancel()
	// cron.Stop() 不阻塞，返回的 ctx 在所有定时任务完成后 Done
	cronCtx := s.cron.Stop()

	// 合并 cron 的完成信号和 immediate 的完成信号
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-cronCtx.Done()
		s.immediateWg.Wait()
		cancel()
	}()
	return ctx
}

// Cron 返回底层 *cron.Cron 实例。
//
// 警告：直接使用底层 cron 添加的任务会绕过 xcron 的分布式锁机制，
// 在多副本场景下可能导致任务重复执行。建议仅用于：
//   - 访问 cron.Entry 等只读信息
//   - 调试和监控目的
//
// 如需添加分布式安全的任务，请使用 AddFunc 或 AddJob 方法。
func (s *cronScheduler) Cron() *cron.Cron {
	return s.cron
}

// Entries 返回所有已注册的任务
func (s *cronScheduler) Entries() []cron.Entry {
	return s.cron.Entries()
}

// Stats 返回执行统计信息
func (s *cronScheduler) Stats() *Stats {
	return s.stats
}

// 确保 cronScheduler 实现了 Scheduler 接口
var _ Scheduler = (*cronScheduler)(nil)
