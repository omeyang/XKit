package xrun

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestGroup_Empty(t *testing.T) {
	g, _ := NewGroup(context.Background())
	if err := g.Wait(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestGroup_SingleService(t *testing.T) {
	var executed atomic.Bool

	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		executed.Store(true)
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if !executed.Load() {
		t.Error("service was not executed")
	}
}

func TestGroup_ServiceError(t *testing.T) {
	expectedErr := errors.New("test error")

	g, _ := NewGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		return expectedErr
	})

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestGroup_ContextCancellation(t *testing.T) {
	var stopped atomic.Bool

	g, ctx := NewGroup(context.Background())

	// 服务 1：等待 context 取消
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		stopped.Store(true)
		return ctx.Err()
	})

	// 服务 2：立即返回错误，触发取消
	g.Go(func(ctx context.Context) error {
		return errors.New("trigger")
	})

	// 期望返回 "trigger" 错误
	if err := g.Wait(); err == nil || err.Error() != "trigger" {
		t.Errorf("expected 'trigger' error, got %v", err)
	}

	// 验证 context 被取消
	select {
	case <-ctx.Done():
		// OK
	default:
		t.Error("context should be canceled")
	}

	if !stopped.Load() {
		t.Error("service 1 was not stopped")
	}
}

func TestGroup_Cancel(t *testing.T) {
	manualErr := errors.New("manual cancel")

	g, ctx := NewGroup(context.Background())

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	// 主动取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(manualErr)
	}()

	err := g.Wait()

	// Cancel(cause) 的原因应该被保留
	if !errors.Is(err, manualErr) {
		t.Errorf("expected manual cancel error, got %v", err)
	}

	select {
	case <-ctx.Done():
		// OK
	default:
		t.Error("context should be canceled")
	}
}

func TestGroup_CancelNil(t *testing.T) {
	g, _ := NewGroup(context.Background())

	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Cancel(nil)
	}()

	// Cancel(nil) 应该返回 nil（正常关闭）
	if err := g.Wait(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestGroup_Context(t *testing.T) {
	g, ctx := NewGroup(context.Background())

	// 验证 Context() 方法返回正确的 context
	if g.Context() != ctx {
		t.Error("Context() should return the group's context")
	}
}

func TestRun_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stopped atomic.Bool
	err := Run(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		stopped.Store(true)
		return ctx.Err()
	})

	// context.Canceled 应该被过滤
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !stopped.Load() {
		t.Error("service was not stopped")
	}
}

func TestRunWithOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stopped atomic.Bool
	logger := slog.Default()
	err := RunWithOptions(ctx, []Option{WithLogger(logger), WithName("test-run")}, func(ctx context.Context) error {
		<-ctx.Done()
		stopped.Store(true)
		return ctx.Err()
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !stopped.Load() {
		t.Error("service was not stopped")
	}
}

func TestTicker(t *testing.T) {
	var count atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(Ticker(10*time.Millisecond, true, func(ctx context.Context) error {
		count.Add(1)
		if count.Load() >= 3 {
			cancel()
		}
		return nil
	}))

	// context.Canceled 会被过滤，期望返回 nil
	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if count.Load() < 3 {
		t.Errorf("expected at least 3 ticks, got %d", count.Load())
	}
}

func TestTicker_NoImmediate(t *testing.T) {
	var count atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(Ticker(10*time.Millisecond, false, func(ctx context.Context) error {
		count.Add(1)
		if count.Load() >= 2 {
			cancel()
		}
		return nil
	}))

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if count.Load() < 2 {
		t.Errorf("expected at least 2 ticks, got %d", count.Load())
	}
}

func TestTicker_InvalidInterval(t *testing.T) {
	for _, interval := range []time.Duration{0, -1, -time.Second} {
		g, _ := NewGroup(context.Background())
		g.Go(Ticker(interval, false, func(ctx context.Context) error {
			return nil
		}))

		err := g.Wait()
		if !errors.Is(err, ErrInvalidInterval) {
			t.Errorf("interval=%v: expected ErrInvalidInterval, got %v", interval, err)
		}
	}
}

func TestTicker_ImmediateError(t *testing.T) {
	expectedErr := errors.New("immediate error")

	g, _ := NewGroup(context.Background())
	g.Go(Ticker(time.Hour, true, func(ctx context.Context) error {
		return expectedErr
	}))

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestTicker_TickError(t *testing.T) {
	expectedErr := errors.New("tick error")
	var count atomic.Int32

	g, _ := NewGroup(context.Background())
	g.Go(Ticker(10*time.Millisecond, false, func(ctx context.Context) error {
		count.Add(1)
		if count.Load() >= 2 {
			return expectedErr
		}
		return nil
	}))

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestTimer(t *testing.T) {
	var executed atomic.Bool

	g, _ := NewGroup(context.Background())
	g.Go(Timer(10*time.Millisecond, func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}))

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !executed.Load() {
		t.Error("timer function was not executed")
	}
}

func TestTimer_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var executed atomic.Bool

	g, _ := NewGroup(ctx)
	g.Go(Timer(time.Hour, func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// context.Canceled 会被过滤，期望返回 nil
	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if executed.Load() {
		t.Error("timer function should not be executed")
	}
}

func TestTimer_InvalidDelay(t *testing.T) {
	for _, delay := range []time.Duration{-1, -time.Second, -time.Hour} {
		g, _ := NewGroup(context.Background())
		g.Go(Timer(delay, func(ctx context.Context) error {
			return nil
		}))

		err := g.Wait()
		if !errors.Is(err, ErrInvalidDelay) {
			t.Errorf("delay=%v: expected ErrInvalidDelay, got %v", delay, err)
		}
	}
}

func TestTimer_ZeroDelay(t *testing.T) {
	// delay=0 应立即执行（有效用例）
	var executed atomic.Bool

	g, _ := NewGroup(context.Background())
	g.Go(Timer(0, func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}))

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !executed.Load() {
		t.Error("timer function was not executed with zero delay")
	}
}

func TestTimer_Error(t *testing.T) {
	expectedErr := errors.New("timer error")

	g, _ := NewGroup(context.Background())
	g.Go(Timer(10*time.Millisecond, func(ctx context.Context) error {
		return expectedErr
	}))

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestHTTPServer(t *testing.T) {
	server := newMockHTTPServer()

	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(HTTPServer(server, time.Second))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !server.shutdownCalled.Load() {
		t.Error("Shutdown was not called")
	}
}

func TestHTTPServer_ExternalShutdown(t *testing.T) {
	// 测试外部直接关闭服务器（非 ctx 驱动）时不会永久阻塞
	server := newMockHTTPServer()

	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(HTTPServer(server, time.Second))

	// 外部触发关闭（不通过 ctx）
	go func() {
		time.Sleep(50 * time.Millisecond)
		server.triggerClose()
	}()

	// 随后取消 ctx，确保不会永久阻塞
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	select {
	case err := <-done:
		// 不应该永久阻塞
		_ = err
	case <-time.After(5 * time.Second):
		t.Fatal("HTTPServer blocked indefinitely on external shutdown")
	}
}

func TestHTTPServer_ListenError(t *testing.T) {
	expectedErr := errors.New("listen error")
	server := &mockHTTPServer{
		listenErr: expectedErr,
		listenCh:  make(chan struct{}),
	}

	g, _ := NewGroup(context.Background())
	g.Go(HTTPServer(server, time.Second))

	// 立即触发关闭
	go func() {
		time.Sleep(10 * time.Millisecond)
		server.triggerClose()
	}()

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestHTTPServer_ShutdownError(t *testing.T) {
	server := &mockHTTPServerWithShutdownError{
		listenCh: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(HTTPServer(server, time.Second))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// shutdown 错误应该被传播
	err := g.Wait()
	if err == nil || err.Error() != "shutdown error" {
		t.Errorf("expected shutdown error, got %v", err)
	}
}

func TestHTTPServer_NoTimeout(t *testing.T) {
	server := newMockHTTPServer()

	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(HTTPServer(server, 0)) // 无超时

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !server.shutdownCalled.Load() {
		t.Error("Shutdown was not called")
	}
}

type mockHTTPServer struct {
	listenErr      error
	shutdownCalled atomic.Bool
	listenCh       chan struct{}
	closeOnce      sync.Once
}

func newMockHTTPServer() *mockHTTPServer {
	return &mockHTTPServer{
		listenCh: make(chan struct{}),
	}
}

func (m *mockHTTPServer) ListenAndServe() error {
	<-m.listenCh
	if m.listenErr != nil {
		return m.listenErr
	}
	return http.ErrServerClosed
}

func (m *mockHTTPServer) Shutdown(ctx context.Context) error {
	m.shutdownCalled.Store(true)
	m.closeOnce.Do(func() {
		close(m.listenCh)
	})
	return nil
}

func (m *mockHTTPServer) triggerClose() {
	m.closeOnce.Do(func() {
		close(m.listenCh)
	})
}

type mockHTTPServerWithShutdownError struct {
	listenCh chan struct{}
}

func (m *mockHTTPServerWithShutdownError) ListenAndServe() error {
	<-m.listenCh
	return http.ErrServerClosed
}

func (m *mockHTTPServerWithShutdownError) Shutdown(ctx context.Context) error {
	close(m.listenCh)
	return errors.New("shutdown error")
}

func TestSignalError(t *testing.T) {
	err := &SignalError{Signal: nil}

	if !errors.Is(err, ErrSignal) {
		t.Error("SignalError should match ErrSignal")
	}

	if errors.Unwrap(err) != ErrSignal {
		t.Error("SignalError should unwrap to ErrSignal")
	}
}

func TestSignalError_Error(t *testing.T) {
	err := &SignalError{Signal: syscall.SIGINT}
	expected := "received signal interrupt"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestSignalError_Error_Nil(t *testing.T) {
	err := &SignalError{Signal: nil}
	expected := "received signal <nil>"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestServiceFunc(t *testing.T) {
	var called atomic.Bool

	svc := ServiceFunc(func(ctx context.Context) error {
		called.Store(true)
		return nil
	})

	err := svc.Run(context.Background())

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called.Load() {
		t.Error("ServiceFunc was not called")
	}
}

func TestGoWithName(t *testing.T) {
	var executed atomic.Bool

	g, _ := NewGroup(context.Background())
	g.GoWithName("test-service", func(ctx context.Context) error {
		executed.Store(true)
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !executed.Load() {
		t.Error("service was not executed")
	}
}

func TestGoWithName_Error(t *testing.T) {
	expectedErr := errors.New("service error")

	g, _ := NewGroup(context.Background())
	g.GoWithName("failing-service", func(ctx context.Context) error {
		return expectedErr
	})

	if err := g.Wait(); err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestGoWithName_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.GoWithName("waiting-service", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// context.Canceled 应该被过滤
	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := NewGroup(ctx)
	g.Go(WaitForDone())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := g.Wait()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMultipleServices(t *testing.T) {
	var count atomic.Int32

	g, _ := NewGroup(context.Background())

	for range 5 {
		g.Go(func(ctx context.Context) error {
			count.Add(1)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if count.Load() != 5 {
		t.Errorf("expected 5 services to run, got %d", count.Load())
	}
}

func TestRunServices(t *testing.T) {
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

	err := RunServices(ctx, svc)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !ran.Load() {
		t.Error("service was not run")
	}
}

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	g, _ := NewGroup(context.Background(), WithLogger(logger))

	g.Go(func(ctx context.Context) error {
		return nil
	})

	if err := g.Wait(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

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
