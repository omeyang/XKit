package xrun

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWithLogger_Nil(t *testing.T) {
	// WithLogger(nil) 应该保持默认值
	g, _ := NewGroup(context.Background(), WithLogger(nil))

	g.Go(func(ctx context.Context) error {
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWithName_Empty(t *testing.T) {
	// WithName("") 应该保持默认值
	g, _ := NewGroup(context.Background(), WithName(""))

	g.Go(func(ctx context.Context) error {
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ----------------------------------------------------------------------------
// 新增测试
// ----------------------------------------------------------------------------

func TestWait_PreserveCancelCause(t *testing.T) {
	customErr := errors.New("custom shutdown reason")

	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(customErr)
	}()

	err := g.Wait()
	if !errors.Is(err, customErr) {
		t.Errorf("expected custom error, got %v", err)
	}
}

func TestRun_SignalError(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	// 模拟发送信号
	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGTERM {
			t.Errorf("expected SIGTERM, got %v", sigErr.Signal)
		}
		if !errors.Is(err, ErrSignal) {
			t.Error("error should match ErrSignal")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to return")
	}
}

func TestRunServices_SignalError(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	svc := ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	done := make(chan error, 1)
	go func() {
		done <- RunServices(ctx, svc)
	}()

	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGINT

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGINT {
			t.Errorf("expected SIGINT, got %v", sigErr.Signal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunServices to return")
	}
}

func TestRunWithOptions_SignalError(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx, []Option{WithName("test")}, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGQUIT

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGQUIT {
			t.Errorf("expected SIGQUIT, got %v", sigErr.Signal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunWithOptions to return")
	}
}

func TestDefaultSignals(t *testing.T) {
	signals := DefaultSignals()
	if len(signals) != 4 {
		t.Errorf("expected 4 signals, got %d", len(signals))
	}

	// 验证修改不影响后续调用
	signals[0] = nil
	signals2 := DefaultSignals()
	if signals2[0] == nil {
		t.Error("DefaultSignals should return a new slice each time")
	}
}

// ----------------------------------------------------------------------------
// Fix 1: context.Canceled 过滤修复
// ----------------------------------------------------------------------------

func TestWait_ServiceContextCanceled(t *testing.T) {
	// 服务自身返回 context.Canceled（非 Group 取消），不应被过滤
	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		return context.Canceled
	})

	err := g.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWait_GroupCancelFiltersCanceled(t *testing.T) {
	// Group 主动取消（Cancel(nil)），context.Canceled 应该被过滤
	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(nil)
	}()

	err := g.Wait()
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestWait_ParentCancelFiltersCanceled(t *testing.T) {
	// 父 context 取消，context.Canceled 应该被过滤
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := g.Wait()
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Fix 2: 信号配置
// ----------------------------------------------------------------------------

func TestWithSignals(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx,
			[]Option{WithSignals([]os.Signal{syscall.SIGINT})},
			func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		)
	}()

	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGINT

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGINT {
			t.Errorf("expected SIGINT, got %v", sigErr.Signal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunWithOptions to return")
	}
}

func TestWithSignals_EmptySlice(t *testing.T) {
	// WithSignals([]os.Signal{}) 应等价于使用默认信号列表，
	// 而非 signal.Notify 的"监听所有信号"行为。
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx,
			[]Option{WithSignals([]os.Signal{})},
			func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		)
	}()

	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGTERM {
			t.Errorf("expected SIGTERM, got %v", sigErr.Signal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunWithOptions to return")
	}
}

func TestWithSignals_DefensiveCopy(t *testing.T) {
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	opt := WithSignals(signals)

	// 修改原始切片
	signals[0] = syscall.SIGHUP

	// 应用 option 并验证不受影响
	opts := defaultOptions()
	opt(opts)
	if opts.signals[0] != syscall.SIGINT {
		t.Error("WithSignals should make a defensive copy")
	}
}

func TestWithoutSignalHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var stopped atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx,
			[]Option{WithoutSignalHandler()},
			func(ctx context.Context) error {
				<-ctx.Done()
				stopped.Store(true)
				return ctx.Err()
			},
		)
	}()

	// 手动取消，无信号处理
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if !stopped.Load() {
			t.Error("service was not stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// ----------------------------------------------------------------------------
// Fix 3: Cancel(cause) 在服务返回 nil 时保留 cause
// ----------------------------------------------------------------------------

func TestWait_CancelCauseWithNilReturn(t *testing.T) {
	// Cancel(cause) 后服务返回 nil（而非 ctx.Err()），cause 不应丢失
	customErr := errors.New("custom cause")

	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return nil // 返回 nil 而非 ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(customErr)
	}()

	err := g.Wait()
	if !errors.Is(err, customErr) {
		t.Errorf("expected custom cause, got %v", err)
	}
}

func TestWait_CancelCauseNoServices(t *testing.T) {
	// Cancel(cause) 但没有注册服务，cause 仍应返回
	customErr := errors.New("custom cause")

	g, _ := NewGroup(context.Background())
	g.Cancel(customErr)

	err := g.Wait()
	if !errors.Is(err, customErr) {
		t.Errorf("expected custom cause, got %v", err)
	}
}

func TestWait_CancelNilWithNilReturn(t *testing.T) {
	// Cancel(nil) 后服务返回 nil，Wait() 应返回 nil
	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(nil)
	}()

	if err := g.Wait(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Fix 4: RunServicesWithOptions
// ----------------------------------------------------------------------------

func TestRunServicesWithOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var ran atomic.Bool

	svc := ServiceFunc(func(ctx context.Context) error {
		ran.Store(true)
		<-ctx.Done()
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := RunServicesWithOptions(ctx, []Option{WithName("test")}, svc)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !ran.Load() {
		t.Error("service was not run")
	}
}

func TestRunServicesWithOptions_SignalError(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	svc := ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	done := make(chan error, 1)
	go func() {
		done <- RunServicesWithOptions(ctx, []Option{WithName("test")}, svc)
	}()

	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		var sigErr *SignalError
		if !errors.As(err, &sigErr) {
			t.Fatalf("expected SignalError, got %v", err)
		}
		if sigErr.Signal != syscall.SIGTERM {
			t.Errorf("expected SIGTERM, got %v", sigErr.Signal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunServicesWithOptions to return")
	}
}

// ----------------------------------------------------------------------------
// nil 参数校验
// ----------------------------------------------------------------------------

func TestTicker_NilFunc(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(Ticker(time.Second, false, nil))

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestTicker_NilFunc_Immediate(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(Ticker(time.Second, true, nil))

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestTimer_NilFunc(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(Timer(time.Second, nil))

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestTimer_NilFunc_ZeroDelay(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(Timer(0, nil))

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestHTTPServer_NilServer(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(HTTPServer(nil, time.Second))

	err := g.Wait()
	if !errors.Is(err, ErrNilServer) {
		t.Errorf("expected ErrNilServer, got %v", err)
	}
}

func TestGo_NilFunc(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.Go(nil)

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestGoWithName_NilFunc(t *testing.T) {
	g, _ := NewGroup(context.Background())
	g.GoWithName("nil-service", nil)

	err := g.Wait()
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestRun_NilFunc(t *testing.T) {
	ctx := t.Context()

	err := RunWithOptions(ctx, []Option{WithoutSignalHandler()}, nil)
	if !errors.Is(err, ErrNilFunc) {
		t.Errorf("expected ErrNilFunc, got %v", err)
	}
}

func TestRunServices_NilService(t *testing.T) {
	ctx := t.Context()

	err := RunServicesWithOptions(ctx, []Option{WithoutSignalHandler()}, nil)
	if !errors.Is(err, ErrNilService) {
		t.Errorf("expected ErrNilService, got %v", err)
	}
}

func TestRunServices_NilServiceDirect(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	ctx := withTestSigChan(context.Background(), sigCh)

	err := RunServices(ctx, nil)
	if !errors.Is(err, ErrNilService) {
		t.Errorf("expected ErrNilService, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// 已取消 context 下的 immediate/zero-delay 行为
// ----------------------------------------------------------------------------

func TestTicker_ImmediateCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	var executed atomic.Bool
	g, _ := NewGroup(ctx)
	g.Go(Ticker(time.Second, true, func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}))

	// context.Canceled 会被过滤（Group 主动取消）
	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if executed.Load() {
		t.Error("fn should not be executed when context is already canceled")
	}
}

func TestTimer_ZeroDelayCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	var executed atomic.Bool
	g, _ := NewGroup(ctx)
	g.Go(Timer(0, func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}))

	// context.Canceled 会被过滤
	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if executed.Load() {
		t.Error("fn should not be executed when context is already canceled")
	}
}

// ----------------------------------------------------------------------------
// nil context
// ----------------------------------------------------------------------------

func TestNewGroup_NilContext(t *testing.T) {
	// nil context 应被归一化为 context.Background()，不 panic
	g, ctx := NewGroup(nil) //nolint:staticcheck // 测试 nil context 归一化行为

	g.Go(func(ctx context.Context) error {
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// context 应该有效
	if ctx == nil {
		t.Error("returned context should not be nil")
	}
}

func TestNewGroup_NilOption(t *testing.T) {
	// nil Option 应被静默跳过，不 panic
	g, _ := NewGroup(context.Background(), nil, WithName("test"), nil)

	g.Go(func(ctx context.Context) error {
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ----------------------------------------------------------------------------
// WithoutSignalHandler 与 WithSignals 优先级
// ----------------------------------------------------------------------------

func TestWithoutSignalHandler_OverridesWithSignals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx,
			[]Option{
				WithSignals([]os.Signal{syscall.SIGINT}),
				WithoutSignalHandler(),
			},
			func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		)
	}()

	// 手动取消（没有信号处理，所以只能手动取消）
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		// WithoutSignalHandler 优先，应返回 nil（正常关闭）
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// ----------------------------------------------------------------------------
// typed-nil 防御
// ----------------------------------------------------------------------------

func TestHTTPServer_TypedNilServer(t *testing.T) {
	var server *http.Server // typed-nil
	g, _ := NewGroup(context.Background())
	g.Go(HTTPServer(server, time.Second))

	err := g.Wait()
	if !errors.Is(err, ErrNilServer) {
		t.Errorf("expected ErrNilServer for typed-nil *http.Server, got %v", err)
	}
}

func TestRunServices_TypedNilServiceFunc(t *testing.T) {
	var svc ServiceFunc // typed-nil
	err := RunServicesWithOptions(
		context.Background(),
		[]Option{WithoutSignalHandler()},
		svc,
	)
	if !errors.Is(err, ErrNilService) {
		t.Errorf("expected ErrNilService for typed-nil ServiceFunc, got %v", err)
	}
}

type stubService struct{}

func (s *stubService) Run(ctx context.Context) error { return nil }

func TestRunServices_TypedNilStructService(t *testing.T) {
	var svc *stubService // typed-nil
	err := RunServicesWithOptions(
		context.Background(),
		[]Option{WithoutSignalHandler()},
		svc,
	)
	if !errors.Is(err, ErrNilService) {
		t.Errorf("expected ErrNilService for typed-nil *stubService, got %v", err)
	}
}
