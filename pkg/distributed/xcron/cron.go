package xcron

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/robfig/cron/v3"
)

// ErrNilJob 表示任务为 nil。
var ErrNilJob = errors.New("xcron: job cannot be nil")

// cronScheduler 基于 robfig/cron/v3 的调度器实现
type cronScheduler struct {
	cron   *cron.Cron
	opts   *schedulerOptions
	locker Locker
	logger Logger
	stats  *Stats // 执行统计

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

	// 警告：配置了分布式锁但未设置任务名，锁将被跳过
	if jobOpts.name == "" {
		if _, isNoop := locker.(*noopLocker); !isNoop {
			if s.logger != nil {
				s.logger.Warn(context.Background(),
					"job has distributed locker but no name; lock will be skipped, use WithName() to enable locking",
					"spec", spec)
			} else {
				log.Printf("[WARN] xcron: job has distributed locker but no name; lock will be skipped, use WithName()")
			}
		}
	}

	// 创建包装器
	wrapper := newJobWrapper(job, locker, s.logger, s.stats, jobOpts)

	// 添加到底层 cron
	id, err := s.cron.AddJob(spec, wrapper)
	if err != nil {
		return 0, fmt.Errorf("xcron: failed to add job: %w", err)
	}

	// 立即执行一次（如果配置了 WithImmediate）
	// 使用 WaitGroup 追踪，确保 Stop() 时能等待完成
	// 创建独立的包装器副本，使用可取消的上下文，以便 Stop() 时能中止
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

// Remove 移除任务
func (s *cronScheduler) Remove(id JobID) {
	s.cron.Remove(id)
}

// Start 启动调度器
func (s *cronScheduler) Start() {
	s.cron.Start()
}

// Stop 优雅停止。
// 会等待所有正在执行的任务完成，包括 WithImmediate 启动的立即执行任务。
// 立即执行任务的上下文会被取消，使其能尽快结束。
func (s *cronScheduler) Stop() context.Context {
	// 取消所有立即执行任务的上下文
	s.immediateCancel()
	ctx := s.cron.Stop()
	// 等待 WithImmediate 启动的立即执行任务完成
	s.immediateWg.Wait()
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
