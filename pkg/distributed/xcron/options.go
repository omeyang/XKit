package xcron

import (
	"time"

	"github.com/robfig/cron/v3"
)

// ===================== Scheduler Options =====================

// schedulerOptions 调度器配置
type schedulerOptions struct {
	locker   Locker         // 默认分布式锁
	logger   Logger         // 日志记录器
	location *time.Location // 时区
	parser   cron.Parser    // cron 表达式解析器
}

// defaultSchedulerOptions 返回默认配置
func defaultSchedulerOptions() *schedulerOptions {
	return &schedulerOptions{
		locker:   NoopLocker(),
		logger:   nil, // 使用内置默认日志
		location: time.Local,
		parser:   cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
	}
}

// SchedulerOption 调度器配置选项
type SchedulerOption func(*schedulerOptions)

// WithLocker 设置默认分布式锁。
//
// 所有任务默认使用此锁，除非任务通过 [WithJobLocker] 单独指定。
// 不设置时默认使用 [NoopLocker]。
//
// 用法：
//
//	locker := xcron.NewRedisLocker(redisClient)
//	scheduler := xcron.New(xcron.WithLocker(locker))
func WithLocker(locker Locker) SchedulerOption {
	return func(o *schedulerOptions) {
		if locker != nil {
			o.locker = locker
		}
	}
}

// WithLogger 设置日志记录器。
//
// 用于记录任务执行状态、锁获取情况等信息。
// 接口兼容 xlog.Logger。
//
// 用法：
//
//	scheduler := xcron.New(xcron.WithLogger(myLogger))
func WithLogger(logger Logger) SchedulerOption {
	return func(o *schedulerOptions) {
		o.logger = logger
	}
}

// WithLocation 设置时区。
//
// cron 表达式中的时间将按此时区解释。默认使用本地时区。
//
// 用法：
//
//	loc, _ := time.LoadLocation("Asia/Shanghai")
//	scheduler := xcron.New(xcron.WithLocation(loc))
func WithLocation(loc *time.Location) SchedulerOption {
	return func(o *schedulerOptions) {
		if loc != nil {
			o.location = loc
		}
	}
}

// WithParser 自定义 cron 表达式解析器。
//
// 用于支持非标准 cron 表达式格式。
//
// 用法：
//
//	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
//	scheduler := xcron.New(xcron.WithParser(parser))
func WithParser(parser cron.Parser) SchedulerOption {
	return func(o *schedulerOptions) {
		o.parser = parser
	}
}

// WithSeconds 启用秒级精度。
//
// 默认 cron 表达式最小精度为分钟，使用此选项后支持秒级：
//
//	scheduler := xcron.New(xcron.WithSeconds())
//	scheduler.AddFunc("*/5 * * * * *", task) // 每 5 秒执行
//
// 等同于：
//
//	WithParser(cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))
func WithSeconds() SchedulerOption {
	return func(o *schedulerOptions) {
		o.parser = cron.NewParser(
			cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		)
	}
}

// ===================== Job Options =====================

// MinLockTTL 是锁 TTL 的最小值。
// 续期间隔为 TTL/3，为保证续期能在锁过期前执行，TTL 至少需要 3 秒。
const MinLockTTL = 3 * time.Second

// jobOptions 任务配置
type jobOptions struct {
	name        string        // 任务名（用作锁 key）
	locker      Locker        // 任务级锁（覆盖全局）
	lockTTL     time.Duration // 锁超时时间
	lockTimeout time.Duration // 锁获取超时时间
	timeout     time.Duration // 执行超时时间
	retry       RetryPolicy   // 重试策略
	backoff     BackoffPolicy // 退避策略
	tracer      Observer      // 链路追踪
	immediate   bool          // 是否立即执行一次
	hooks       []Hook        // 执行钩子
}

// defaultJobOptions 返回默认任务配置
func defaultJobOptions() *jobOptions {
	return &jobOptions{
		name:        "",
		locker:      nil, // nil 表示使用全局锁
		lockTTL:     5 * time.Minute,
		lockTimeout: 5 * time.Second, // 默认锁获取超时，防止底层存储响应慢
		timeout:     0,               // 0 表示无超时
		retry:       nil,
		backoff:     nil,
		tracer:      nil,
	}
}

// JobOption 任务配置选项
type JobOption func(*jobOptions)

// WithName 设置任务名。
//
// 任务名用作分布式锁的 key，在同一调度器内必须唯一。
// 当调度器配置了分布式锁（非 NoopLocker）时，必须设置任务名，
// 否则 AddFunc/AddJob 返回 [ErrMissingName]。
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task, xcron.WithName("my-task"))
func WithName(name string) JobOption {
	return func(o *jobOptions) {
		o.name = name
	}
}

// WithJobLocker 设置任务级分布式锁。
//
// 覆盖调度器级别的默认锁设置。用于部分任务需要特殊锁策略的场景。
//
// 用法：
//
//	// 全局使用 Redis 锁，但这个任务不需要锁
//	scheduler.AddFunc("@every 1s", localTask, xcron.WithJobLocker(xcron.NoopLocker()))
func WithJobLocker(locker Locker) JobOption {
	return func(o *jobOptions) {
		o.locker = locker
	}
}

// WithLockTTL 设置锁超时时间。
//
// 锁在此时间后自动过期，防止任务崩溃导致死锁。
// 应设置为大于任务最大执行时间的值。默认 5 分钟。
// 最小值为 [MinLockTTL]（3秒），小于此值会被自动调整。
//
// 对于长时间运行的任务，xcron 会自动续期锁。
//
// 用法：
//
//	scheduler.AddFunc("0 2 * * *", longTask, xcron.WithLockTTL(30*time.Minute))
func WithLockTTL(ttl time.Duration) JobOption {
	return func(o *jobOptions) {
		if ttl > 0 {
			// 强制最小 TTL，确保续期间隔（TTL/3）至少为 1 秒
			if ttl < MinLockTTL {
				ttl = MinLockTTL
			}
			o.lockTTL = ttl
		}
	}
}

// WithTimeout 设置任务执行超时。
//
// 任务执行超过此时间将被取消（ctx.Done()）。默认无超时。
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task, xcron.WithTimeout(30*time.Second))
func WithTimeout(timeout time.Duration) JobOption {
	return func(o *jobOptions) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// WithLockTimeout 设置锁获取超时时间。
//
// 调用 TryLock 时如果底层存储（Redis/K8s API）响应慢，
// 超过此时间将放弃获取锁，避免 goroutine 长时间阻塞。默认 5 秒。
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task,
//	    xcron.WithName("my-task"),
//	    xcron.WithLockTimeout(5*time.Second),
//	)
func WithLockTimeout(timeout time.Duration) JobOption {
	return func(o *jobOptions) {
		if timeout > 0 {
			o.lockTimeout = timeout
		}
	}
}

// WithRetry 设置重试策略。
//
// 任务执行失败时按此策略重试。需要配合 [WithBackoff] 设置重试间隔。
// 接口兼容 xretry.RetryPolicy。
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task,
//	    xcron.WithRetry(xretry.NewFixedRetry(3)),
//	    xcron.WithBackoff(xretry.NewExponentialBackoff()),
//	)
func WithRetry(policy RetryPolicy) JobOption {
	return func(o *jobOptions) {
		o.retry = policy
	}
}

// WithBackoff 设置退避策略。
//
// 配合 [WithRetry] 使用，定义重试之间的等待时间。
// 接口兼容 xretry.BackoffPolicy。
func WithBackoff(policy BackoffPolicy) JobOption {
	return func(o *jobOptions) {
		o.backoff = policy
	}
}

// WithTracer 设置链路追踪。
//
// 每次任务执行会创建一个 span，记录执行时间和结果。
// 接口兼容 xmetrics.Observer。
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task, xcron.WithTracer(myObserver))
func WithTracer(tracer Observer) JobOption {
	return func(o *jobOptions) {
		o.tracer = tracer
	}
}

// WithImmediate 设置任务在注册后立即执行一次。
//
// 立即执行会应用同样的锁、超时、重试逻辑。
// 如果立即执行失败，任务仍会被注册到调度器继续按计划执行。
// 立即执行是异步的，不会阻塞 AddFunc/AddJob 返回。
//
// 典型场景：
//   - 服务启动时需要立即同步一次数据
//   - 定时清理任务启动时先清理一次
//   - 缓存预热任务需要立即加载
//
// 用法：
//
//	scheduler.AddFunc("@every 1h", syncTask,
//	    xcron.WithName("data-sync"),
//	    xcron.WithImmediate(),
//	)
func WithImmediate() JobOption {
	return func(o *jobOptions) {
		o.immediate = true
	}
}

// WithHook 添加单个任务执行钩子。
//
// 钩子在任务执行前后被调用，用于注入自定义逻辑。
// 可多次调用以添加多个钩子，按添加顺序执行。
//
// 执行顺序：
//   - BeforeJob: hook1 → hook2 → hook3
//   - AfterJob: hook3 → hook2 → hook1 （逆序，类似 defer）
//
// 典型用途：
//   - 日志记录
//   - 指标上报
//   - 告警通知
//   - 审计追踪
//
// 用法：
//
//	scheduler.AddFunc("@every 1m", task,
//	    xcron.WithName("my-task"),
//	    xcron.WithHook(&myMetricsHook{}),
//	    xcron.WithHook(&myAlertHook{}),
//	)
func WithHook(hook Hook) JobOption {
	return func(o *jobOptions) {
		if hook != nil {
			o.hooks = append(o.hooks, hook)
		}
	}
}

// WithHooks 批量添加任务执行钩子。
//
// 等同于多次调用 [WithHook]，按参数顺序添加钩子。
//
// 用法：
//
//	hooks := []xcron.Hook{metricsHook, alertHook, auditHook}
//	scheduler.AddFunc("@every 1m", task,
//	    xcron.WithName("my-task"),
//	    xcron.WithHooks(hooks...),
//	)
func WithHooks(hooks ...Hook) JobOption {
	return func(o *jobOptions) {
		for _, hook := range hooks {
			if hook != nil {
				o.hooks = append(o.hooks, hook)
			}
		}
	}
}
