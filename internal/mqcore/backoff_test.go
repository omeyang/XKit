package mqcore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

func TestRunConsumeLoop_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var count atomic.Int32
	consume := func(ctx context.Context) error {
		count.Add(1)
		return nil
	}

	err := RunConsumeLoop(ctx, consume)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	if count.Load() < 1 {
		t.Errorf("expected at least 1 call, got %d", count.Load())
	}
}

func TestRunConsumeLoop_ErrorWithBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var count atomic.Int32
	var errorCount atomic.Int32
	testErr := errors.New("test error")

	consume := func(ctx context.Context) error {
		count.Add(1)
		return testErr
	}

	onError := func(err error) {
		errorCount.Add(1)
	}

	// 使用固定退避来简化测试
	backoff := xretry.NewFixedBackoff(50 * time.Millisecond)

	err := RunConsumeLoop(ctx, consume,
		WithBackoff(backoff),
		WithOnError(onError),
	)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// 500ms / 50ms = 最多约 10 次调用
	if count.Load() < 2 {
		t.Errorf("expected at least 2 calls, got %d", count.Load())
	}
	if count.Load() > 15 {
		t.Errorf("expected at most 15 calls, got %d", count.Load())
	}
	if errorCount.Load() != count.Load() {
		t.Errorf("error count mismatch: errors=%d, calls=%d", errorCount.Load(), count.Load())
	}
}

func TestRunConsumeLoop_ResetOnSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var count atomic.Int32
	consume := func(ctx context.Context) error {
		n := count.Add(1)
		// 前两次失败，之后成功
		if n <= 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	// 使用较短的固定退避
	backoff := xretry.NewFixedBackoff(10 * time.Millisecond)

	err := RunConsumeLoop(ctx, consume, WithBackoff(backoff))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// 应该有很多次成功调用
	if count.Load() < 5 {
		t.Errorf("expected at least 5 calls, got %d", count.Load())
	}
}

func TestRunConsumeLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var count atomic.Int32
	consume := func(ctx context.Context) error {
		n := count.Add(1)
		if n >= 3 {
			cancel()
		}
		return errors.New("error")
	}

	backoff := xretry.NewFixedBackoff(10 * time.Millisecond)
	err := RunConsumeLoop(ctx, consume, WithBackoff(backoff))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDefaultBackoff(t *testing.T) {
	backoff := DefaultBackoff()
	if backoff == nil {
		t.Fatal("DefaultBackoff returned nil")
	}

	// 验证是 ExponentialBackoff 类型
	_, ok := backoff.(*xretry.ExponentialBackoff)
	if !ok {
		t.Errorf("expected *xretry.ExponentialBackoff, got %T", backoff)
	}

	// 验证默认延迟
	delay := backoff.NextDelay(1)
	// 默认 100ms ± 10% jitter
	if delay < 90*time.Millisecond || delay > 110*time.Millisecond {
		t.Errorf("expected delay around 100ms, got %v", delay)
	}
}

func TestWithBackoff_NilIgnored(t *testing.T) {
	opts := &ConsumeLoopOptions{
		Backoff: DefaultBackoff(),
	}
	original := opts.Backoff

	WithBackoff(nil)(opts)

	if opts.Backoff != original {
		t.Error("nil backoff should be ignored")
	}
}

func TestWithOnError_NilIgnored(t *testing.T) {
	original := func(_ error) {}
	opts := &ConsumeLoopOptions{
		OnError: original,
	}

	WithOnError(nil)(opts)

	if opts.OnError == nil {
		t.Error("nil onError should be ignored, existing callback should be preserved")
	}
}

func TestRunConsumeLoop_NilConsume(t *testing.T) {
	err := RunConsumeLoop(context.Background(), nil)
	if !errors.Is(err, ErrNilHandler) {
		t.Errorf("expected ErrNilHandler, got %v", err)
	}
}

func TestRunConsumeLoop_NilContext(t *testing.T) {
	// nil ctx 应归一化为 context.Background()，不 panic。
	var ctxWasNil atomic.Bool
	var count atomic.Int32
	done := make(chan struct{})

	go func() {
		defer close(done)
		//nolint:staticcheck // SA1012: 此测试专门验证 nil ctx 归一化行为
		RunConsumeLoop(nil, func(innerCtx context.Context) error { //nolint:errcheck // 测试 goroutine 中无法传递错误
			if innerCtx == nil {
				ctxWasNil.Store(true)
			}
			count.Add(1)
			return nil
		})
	}()

	// 等待至少执行 3 次 consume，证明 nil ctx 已归一化且循环正常运行
	deadline := time.After(2 * time.Second)
	for count.Load() < 3 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for consume calls")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if ctxWasNil.Load() {
		t.Error("ctx inside consume should not be nil after normalization")
	}
}

// mockResettableBackoff 实现 ResettableBackoff 接口用于测试
type mockResettableBackoff struct {
	delay      time.Duration
	resetCount atomic.Int32
}

func (m *mockResettableBackoff) NextDelay(_ int) time.Duration {
	return m.delay
}

func (m *mockResettableBackoff) Reset() {
	m.resetCount.Add(1)
}

func TestRunConsumeLoop_ResettableBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var count atomic.Int32
	consume := func(ctx context.Context) error {
		n := count.Add(1)
		// 第一次失败，之后成功
		if n == 1 {
			return errors.New("temporary error")
		}
		return nil
	}

	backoff := &mockResettableBackoff{delay: 10 * time.Millisecond}

	err := RunConsumeLoop(ctx, consume, WithBackoff(backoff))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// 验证 Reset() 被调用（每次成功消费后都应该调用）
	resetCount := backoff.resetCount.Load()
	if resetCount < 1 {
		t.Errorf("expected Reset() to be called at least once, got %d calls", resetCount)
	}
}
