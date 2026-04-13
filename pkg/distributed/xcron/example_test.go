package xcron_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/distributed/xcron"
)

func Example_basic() {
	// 创建调度器（单副本场景，使用默认 NoopLocker）
	scheduler := xcron.New()

	var count atomic.Int32
	// 添加定时任务
	_, err := scheduler.AddFunc("@every 1s", func(ctx context.Context) error {
		c := count.Add(1)
		if c <= 2 {
			fmt.Println("task executed")
		}
		return nil
	}, xcron.WithName("example-task"))
	if err != nil {
		panic(err)
	}

	// 启动调度器
	scheduler.Start()

	// 运行一段时间
	time.Sleep(2200 * time.Millisecond)

	// 优雅停止
	ctx := scheduler.Stop()
	<-ctx.Done()

	// Output:
	// task executed
	// task executed
}

func Example_withTimeout() {
	scheduler := xcron.New()

	var executed atomic.Bool
	// 添加带超时的任务
	if _, err := scheduler.AddFunc("@every 1s", func(ctx context.Context) error {
		if executed.Load() {
			return nil // 只打印一次
		}
		executed.Store(true)
		// 检查 context 是否有超时
		if deadline, ok := ctx.Deadline(); ok {
			fmt.Printf("task has deadline: %v\n", deadline.After(time.Now()))
		}
		return nil
	},
		xcron.WithName("timeout-task"),
		xcron.WithTimeout(5*time.Second),
	); err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(1200 * time.Millisecond)
	scheduler.Stop()

	// Output:
	// task has deadline: true
}

func Example_withJobInterface() {
	// 使用 JobFunc 适配器可以将普通函数转换为 Job 接口
	// 实际使用时也可以定义结构体实现 Job 接口

	scheduler := xcron.New()

	var executed atomic.Bool
	// 使用 JobFunc 适配器
	job := xcron.JobFunc(func(ctx context.Context) error {
		if !executed.Load() {
			executed.Store(true)
			fmt.Println("MyJob executed")
		}
		return nil
	})

	if _, err := scheduler.AddJob("@every 1s", job, xcron.WithName("my-job")); err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(1200 * time.Millisecond)
	scheduler.Stop()

	// Output:
	// MyJob executed
}

func Example_multipleJobs() {
	scheduler := xcron.New()

	var fastCount, slowCount atomic.Int32
	// 添加多个任务
	if _, err := scheduler.AddFunc("@every 500ms", func(ctx context.Context) error {
		fastCount.Add(1)
		return nil
	}, xcron.WithName("fast-task")); err != nil {
		panic(err)
	}

	if _, err := scheduler.AddFunc("@every 1s", func(ctx context.Context) error {
		slowCount.Add(1)
		return nil
	}, xcron.WithName("slow-task")); err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(1200 * time.Millisecond)
	scheduler.Stop()

	// 快任务应该执行更多次
	fmt.Printf("fast runs more than slow: %v\n", fastCount.Load() >= slowCount.Load())

	// Output:
	// fast runs more than slow: true
}

func Example_removeJob() {
	scheduler := xcron.New()

	var executed atomic.Bool
	// 添加任务并保存 ID
	id, err := scheduler.AddFunc("@every 500ms", func(ctx context.Context) error {
		if !executed.Swap(true) {
			fmt.Println("will be removed")
		}
		return nil
	}, xcron.WithName("removable-task"))
	if err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(1200 * time.Millisecond)

	// 移除任务
	scheduler.Remove(id)
	fmt.Println("job removed")

	time.Sleep(600 * time.Millisecond)
	scheduler.Stop()

	// Output:
	// will be removed
	// job removed
}

func Example_cronExpression() {
	scheduler := xcron.New()

	// 各种 cron 表达式示例（使用切片保证输出顺序）
	type cronExample struct {
		expr string
		desc string
	}
	expressions := []cronExample{
		{"@every 1s", "每秒"},
		{"@every 1m", "每分钟"},
		{"@hourly", "每小时"},
		{"@daily", "每天午夜"},
		{"0 * * * *", "每小时第 0 分钟"},
		{"30 9 * * 1", "每周一上午 9:30"},
		{"0 0 1 * *", "每月 1 号午夜"},
		{"0 0 1 1 *", "每年 1 月 1 日午夜"},
	}

	for _, e := range expressions {
		_, err := scheduler.AddFunc(e.expr, func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			fmt.Printf("%s (%s): invalid\n", e.expr, e.desc)
		} else {
			fmt.Printf("%s (%s): valid\n", e.expr, e.desc)
		}
	}

	// Output:
	// @every 1s (每秒): valid
	// @every 1m (每分钟): valid
	// @hourly (每小时): valid
	// @daily (每天午夜): valid
	// 0 * * * * (每小时第 0 分钟): valid
	// 30 9 * * 1 (每周一上午 9:30): valid
	// 0 0 1 * * (每月 1 号午夜): valid
	// 0 0 1 1 * (每年 1 月 1 日午夜): valid
}

func Example_withSeconds() {
	// 启用秒级精度
	scheduler := xcron.New(xcron.WithSeconds())

	// 现在可以使用 6 段 cron 表达式（秒 分 时 日 月 周）
	_, err := scheduler.AddFunc("*/2 * * * * *", func(ctx context.Context) error {
		fmt.Println("every 2 seconds")
		return nil
	}, xcron.WithName("seconds-task"))

	if err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(5 * time.Second)
	scheduler.Stop()
}

func Example_accessUnderlyingCron() {
	scheduler := xcron.New()

	// 获取底层 *cron.Cron，使用原生能力
	c := scheduler.Cron()

	var executed bool
	// 例如：使用原生 AddFunc（不带分布式锁）
	if _, err := c.AddFunc("@every 1s", func() {
		if !executed {
			executed = true
			fmt.Println("native cron job")
		}
	}); err != nil {
		panic(err)
	}

	scheduler.Start()
	time.Sleep(1200 * time.Millisecond)
	scheduler.Stop()

	// Output:
	// native cron job
}
