package xcron

import (
	"context"

	"github.com/robfig/cron/v3"
)

// Scheduler 定时任务调度器接口。
//
// 封装 robfig/cron/v3，增加分布式锁支持。
// 使用 [New] 创建默认实现。
type Scheduler interface {
	// AddFunc 添加函数任务。
	//
	// spec 是 cron 表达式，如 "@every 1m" 或 "0 * * * *"。
	// cmd 是任务函数，接收 context 用于超时控制和追踪。
	// opts 是任务选项，如 WithName、WithTimeout 等。
	//
	// 返回 JobID 用于后续移除任务。
	//
	// 用法：
	//
	//	id, err := scheduler.AddFunc("@every 1m", func(ctx context.Context) error {
	//	    return doSomething(ctx)
	//	}, xcron.WithName("my-task"))
	AddFunc(spec string, cmd func(ctx context.Context) error, opts ...JobOption) (JobID, error)

	// AddJob 添加实现了 [Job] 接口的任务。
	//
	// 用法：
	//
	//	id, err := scheduler.AddJob("@daily", &MyJob{}, xcron.WithName("daily-job"))
	AddJob(spec string, job Job, opts ...JobOption) (JobID, error)

	// Remove 移除任务。
	//
	// 移除后任务将不再被调度，正在执行的任务不受影响。
	Remove(id JobID)

	// Start 启动调度器（非阻塞）。
	//
	// 调用后调度器开始按计划执行任务。
	// 重复调用无效果。
	Start()

	// Stop 优雅停止调度器。
	//
	// 停止接受新的任务调度，返回的 context 在所有运行中的任务完成后 Done。
	//
	// 用法：
	//
	//	ctx := scheduler.Stop()
	//	<-ctx.Done() // 等待所有任务完成
	Stop() context.Context

	// Cron 返回底层 *cron.Cron。
	//
	// 用于访问原生能力，如添加不带分布式锁的任务。
	Cron() *cron.Cron

	// Entries 返回所有已注册的任务。
	Entries() []cron.Entry

	// Stats 返回执行统计信息。
	//
	// 返回的 Stats 对象是线程安全的，可以在任务执行期间安全读取。
	// 统计信息包括总执行次数、成功/失败次数、执行时长等。
	//
	// 用法：
	//
	//	stats := scheduler.Stats()
	//	fmt.Printf("总执行: %d, 成功: %d, 失败: %d\n",
	//	    stats.TotalExecutions(),
	//	    stats.SuccessCount(),
	//	    stats.FailureCount())
	Stats() *Stats
}
