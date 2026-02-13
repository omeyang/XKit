package xrun

import (
	"context"
	"errors"
	"testing"
	"time"
)

// FuzzGroup_ServiceCount 测试不同数量的服务
func FuzzGroup_ServiceCount(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(1))
	f.Add(uint8(10))
	f.Add(uint8(50))
	f.Add(uint8(100))

	f.Fuzz(func(t *testing.T, n uint8) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g, _ := NewGroup(ctx)

		for range int(n) {
			g.Go(func(ctx context.Context) error {
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			t.Errorf("unexpected error with %d services: %v", n, err)
		}
	})
}

// FuzzTicker_Interval 测试不同的 ticker 间隔
func FuzzTicker_Interval(f *testing.F) {
	f.Add(int64(1))       // 1 纳秒
	f.Add(int64(1000))    // 1 微秒
	f.Add(int64(1000000)) // 1 毫秒

	f.Fuzz(func(t *testing.T, intervalNs int64) {
		// 限制最大间隔为 10 毫秒，避免测试太慢
		if intervalNs > 10_000_000 {
			intervalNs = 10_000_000
		}
		// 限制最小间隔为 1 纳秒
		if intervalNs <= 0 {
			intervalNs = 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		g, _ := NewGroup(ctx)
		count := 0
		g.Go(Ticker(time.Duration(intervalNs), true, func(ctx context.Context) error {
			count++
			if count >= 3 {
				cancel()
			}
			return nil
		}))

		// 期望正常退出，context.Canceled 会被过滤
		if err := g.Wait(); err != nil {
			t.Errorf("unexpected error with interval %d ns: %v", intervalNs, err)
		}
	})
}

// FuzzTimer_Delay 测试不同的 timer 延迟
func FuzzTimer_Delay(f *testing.F) {
	f.Add(int64(-1000000)) // -1 毫秒（负延迟）
	f.Add(int64(0))        // 0 纳秒
	f.Add(int64(1))        // 1 纳秒
	f.Add(int64(1000))     // 1 微秒
	f.Add(int64(1000000))  // 1 毫秒

	f.Fuzz(func(t *testing.T, delayNs int64) {
		// 限制最大延迟为 10 毫秒，避免测试太慢
		if delayNs > 10_000_000 {
			delayNs = 10_000_000
		}

		delay := time.Duration(delayNs)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		g, _ := NewGroup(ctx)
		executed := false
		g.Go(Timer(delay, func(ctx context.Context) error {
			executed = true
			return nil
		}))

		err := g.Wait()

		if delay < 0 {
			// 负延迟应返回 ErrInvalidDelay
			if !errors.Is(err, ErrInvalidDelay) {
				t.Errorf("expected ErrInvalidDelay with delay %d ns, got %v", delayNs, err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error with delay %d ns: %v", delayNs, err)
			}
			if !executed {
				t.Errorf("timer was not executed with delay %d ns", delayNs)
			}
		}
	})
}

// FuzzSignalError_Message 测试 SignalError 的错误消息
func FuzzSignalError_Message(f *testing.F) {
	f.Add("test signal")
	f.Add("")
	f.Add("SIGINT")
	f.Add("SIGTERM")

	f.Fuzz(func(t *testing.T, signalName string) {
		// 使用自定义 signal 类型来测试
		sig := testSignal(signalName)
		err := &SignalError{Signal: sig}

		// 验证错误消息格式
		expected := "received signal " + signalName
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}

		// 验证 errors.Is
		if !errors.Is(err, ErrSignal) {
			t.Error("SignalError should match ErrSignal")
		}
	})
}

// testSignal 是用于测试的自定义 Signal 实现
type testSignal string

func (s testSignal) Signal() {}

func (s testSignal) String() string {
	return string(s)
}

// FuzzGroup_CancelConcurrent 测试并发取消
func FuzzGroup_CancelConcurrent(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(5), uint8(3))
	f.Add(uint8(10), uint8(5))

	f.Fuzz(func(t *testing.T, serviceCount, cancelCount uint8) {
		if serviceCount == 0 {
			serviceCount = 1
		}
		if cancelCount == 0 {
			cancelCount = 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		g, _ := NewGroup(ctx)

		// 启动多个服务
		for range int(serviceCount) {
			g.Go(func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
		}

		// 并发调用 Cancel
		for range int(cancelCount) {
			go func() {
				g.Cancel(nil)
			}()
		}

		// 应该正常返回，context.Canceled 会被过滤
		if err := g.Wait(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// FuzzHTTPServer_ShutdownTimeout 测试不同的关闭超时
func FuzzHTTPServer_ShutdownTimeout(f *testing.F) {
	f.Add(int64(0))          // 无超时
	f.Add(int64(1000000))    // 1 毫秒
	f.Add(int64(1000000000)) // 1 秒

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		// 限制最大超时为 100 毫秒，避免测试太慢
		if timeoutNs > 100_000_000 {
			timeoutNs = 100_000_000
		}
		// 确保非负
		if timeoutNs < 0 {
			timeoutNs = 0
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		server := newMockHTTPServer()

		g, _ := NewGroup(ctx)
		g.Go(HTTPServer(server, time.Duration(timeoutNs)))

		// 立即取消
		cancel()

		if err := g.Wait(); err != nil {
			t.Errorf("unexpected error with timeout %d ns: %v", timeoutNs, err)
		}
	})
}
