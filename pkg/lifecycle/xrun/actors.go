package xrun

import (
	"context"
	"os"
	"syscall"
	"time"
)

// DefaultSignals 返回默认监听的系统信号列表。
//
// 包含 SIGHUP、SIGINT、SIGTERM、SIGQUIT。注意 SIGHUP 在终端断开
// （如 SSH 断连）时会触发，容器化部署中通常无此问题。如需排除 SIGHUP，
// 可通过 [WithSignals] 自定义信号列表。
//
// 每次调用返回新的切片，调用者可安全修改。
func DefaultSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}
}

// 设计决策: testSigChanKey/testSigChan/withTestSigChan 定义在非测试文件中，
// 因为 runGroup（生产代码）调用 testSigChan 从 context 获取测试通道。
// 这避免了测试中发送真实系统信号（可能影响进程或被 CI 拦截），
// 代价是生产代码包含少量测试辅助逻辑（仅 context.Value 查找，零开销）。

// testSigChanKey 用于在测试中通过 context 注入信号通道。
type testSigChanKey struct{}

// testSigChan 从 context 中获取测试信号通道（生产环境返回 nil）。
func testSigChan(ctx context.Context) <-chan os.Signal {
	c, ok := ctx.Value(testSigChanKey{}).(<-chan os.Signal)
	if !ok {
		return nil
	}
	return c
}

// withTestSigChan 在 context 中注入测试信号通道。
func withTestSigChan(ctx context.Context, c <-chan os.Signal) context.Context {
	return context.WithValue(ctx, testSigChanKey{}, c)
}

// ----------------------------------------------------------------------------
// 服务函数（推荐使用）
// ----------------------------------------------------------------------------

// Ticker 返回周期性执行任务的服务函数。
//
// interval 必须为正数，否则返回的服务函数会返回 ErrInvalidInterval。
// fn 会在每个周期执行。当 ctx 被取消时，返回 ctx.Err()。
// immediate 为 true 时，会在启动时立即执行一次。
//
// 示例：
//
//	g.Go(xrun.Ticker(time.Minute, true, func(ctx context.Context) error {
//	    return doPeriodicWork(ctx)
//	}))
func Ticker(interval time.Duration, immediate bool, fn func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if interval <= 0 {
			return ErrInvalidInterval
		}
		if fn == nil {
			return ErrNilFunc
		}

		// 设计决策: 立即执行前先检查 ctx.Err()，确保已取消的 context
		// 不会触发业务副作用（如发送消息、写库）。
		if immediate {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := fn(ctx); err != nil {
				return err
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := fn(ctx); err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// Timer 返回延迟执行一次任务的服务函数。
//
// delay 不能为负数，否则返回的服务函数会返回 ErrInvalidDelay。
// delay 为 0 时表示立即执行（等效于直接调用 fn）。
// 当 ctx 被取消时，返回 ctx.Err()。
//
// 示例：
//
//	g.Go(xrun.Timer(5*time.Second, func(ctx context.Context) error {
//	    return doDelayedWork(ctx)
//	}))
func Timer(delay time.Duration, fn func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// 设计决策: delay < 0 校验与 Ticker 的 interval <= 0 校验保持一致性。
		// delay == 0 是有效用例（立即触发），而 Ticker 的 interval == 0
		// 会导致 time.NewTicker panic，所以两者的边界值不同。
		if delay < 0 {
			return ErrInvalidDelay
		}
		if fn == nil {
			return ErrNilFunc
		}
		// 设计决策: 零延迟执行前先检查 ctx.Err()，与 Ticker immediate 分支对齐。
		if delay == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fn(ctx)
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			return fn(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WaitForDone 返回等待 context 取消的服务函数。
//
// 这是一个占位服务，用于保持 Group 运行直到收到取消信号。
//
// 示例：
//
//	g.Go(xrun.WaitForDone())
func WaitForDone() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
}
