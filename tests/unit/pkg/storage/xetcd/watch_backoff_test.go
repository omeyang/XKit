package xetcd

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// TestNextBackoff 测试退避时间计算逻辑。
// nextBackoff 包含 ±20% jitter，因此结果在 [base*0.8, base*1.2) 范围内。
// 对于被 MaxBackoff 截断的场景，结果 ≤ MaxBackoff。
func TestNextBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	tests := []struct {
		name    string
		current time.Duration
		wantMin time.Duration // base * 0.8（含 jitter 下界）
		wantMax time.Duration // min(base * 1.2, MaxBackoff)（含 jitter 上界）
	}{
		{"1s to ~2s", 1 * time.Second, 1600 * time.Millisecond, 2400 * time.Millisecond},
		{"2s to ~4s", 2 * time.Second, 3200 * time.Millisecond, 4800 * time.Millisecond},
		{"15s capped at 30s", 15 * time.Second, 24 * time.Second, 30 * time.Second},
		{"20s capped at 30s", 20 * time.Second, 30 * time.Second, 30 * time.Second},
		{"30s stays 30s", 30 * time.Second, 30 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBackoff(tt.current, cfg)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("nextBackoff(%v) = %v, want in [%v, %v]",
					tt.current, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestNextBackoff_OverflowSaturation 验证极大 Duration 不会因 float64→int64 溢出变负。
func TestNextBackoff_OverflowSaturation(t *testing.T) {
	cfg := RetryConfig{
		MaxBackoff:        math.MaxInt64,
		BackoffMultiplier: 2.0,
	}
	got := nextBackoff(math.MaxInt64, cfg)
	if got < 0 {
		t.Errorf("nextBackoff(MaxInt64) = %v, want non-negative", got)
	}
}

// TestAddJitter 测试 addJitter 函数。
func TestAddJitter(t *testing.T) {
	t.Run("zero duration", func(t *testing.T) {
		got := addJitter(0)
		if got != 0 {
			t.Errorf("addJitter(0) = %v, want 0", got)
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		got := addJitter(-time.Second)
		if got != -time.Second {
			t.Errorf("addJitter(-1s) = %v, want -1s", got)
		}
	})

	t.Run("jitter range", func(t *testing.T) {
		d := 10 * time.Second
		minExpected := 8 * time.Second  // d * 0.8
		maxExpected := 12 * time.Second // d * 1.2

		for range 100 {
			got := addJitter(d)
			if got < minExpected || got > maxExpected {
				t.Errorf("addJitter(%v) = %v, want in [%v, %v]",
					d, got, minExpected, maxExpected)
			}
		}
	})
}

// TestSleepWithCancel_Normal 测试正常睡眠等待定时器触发。
func TestSleepWithCancel_Normal(t *testing.T) {
	done := make(chan struct{})
	start := time.Now()
	sleepWithCancel(context.Background(), 10*time.Millisecond, done)
	elapsed := time.Since(start)

	if elapsed < 5*time.Millisecond {
		t.Errorf("sleepWithCancel() returned too quickly: %v", elapsed)
	}
}

// TestSleepWithCancel_ContextCancel 测试 context 取消时立即返回。
func TestSleepWithCancel_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	sleepWithCancel(ctx, 10*time.Second, done)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("sleepWithCancel() should return quickly on cancel, took %v", elapsed)
	}
}

// TestSleepWithCancel_DoneClosed 测试 done 通道关闭时立即返回。
func TestSleepWithCancel_DoneClosed(t *testing.T) {
	done := make(chan struct{})

	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()

	start := time.Now()
	sleepWithCancel(context.Background(), 10*time.Second, done)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("sleepWithCancel() should return quickly on done close, took %v", elapsed)
	}
}

// TestShouldStopWatch_ContextDone 测试 context 已取消时返回 true。
func TestShouldStopWatch_ContextDone(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !c.shouldStopWatch(ctx) {
		t.Error("shouldStopWatch() should return true when context is done")
	}
}

// TestShouldStopWatch_ClientClosed 测试客户端已关闭时返回 true。
func TestShouldStopWatch_ClientClosed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	if !c.shouldStopWatch(context.Background()) {
		t.Error("shouldStopWatch() should return true when client is closed")
	}
}

// TestShouldStopWatch_Normal 测试正常情况返回 false。
func TestShouldStopWatch_Normal(t *testing.T) {
	c := &Client{}

	if c.shouldStopWatch(context.Background()) {
		t.Error("shouldStopWatch() should return false for normal client")
	}
}

// TestBuildRetryWatchOptions_WithLastRevision 测试有 lastRevision 时追加 WithRevision 选项。
func TestBuildRetryWatchOptions_WithLastRevision(t *testing.T) {
	c := &Client{}
	state := &watchRetryState{lastRevision: 100}

	opts := c.buildRetryWatchOptions(nil, state)

	// startRev = 100 + 1 = 101 > 1，应当追加 WithRevision
	if len(opts) == 0 {
		t.Error("buildRetryWatchOptions() should add WithRevision when lastRevision > 0")
	}
}

// TestBuildRetryWatchOptions_WithCompactRevision 测试 compactRevision 大于 lastRevision+1 时使用 compactRevision。
func TestBuildRetryWatchOptions_WithCompactRevision(t *testing.T) {
	c := &Client{}
	state := &watchRetryState{
		lastRevision:    50,
		compactRevision: 200,
	}

	opts := c.buildRetryWatchOptions(nil, state)

	// startRev = max(51, 200) = 200 > 1，应追加 WithRevision
	if len(opts) == 0 {
		t.Error("buildRetryWatchOptions() should add WithRevision for compaction recovery")
	}
}

// TestBuildRetryWatchOptions_NoRevision 测试 lastRevision=0 且无 compactRevision 时不追加选项。
func TestBuildRetryWatchOptions_NoRevision(t *testing.T) {
	c := &Client{}
	state := &watchRetryState{lastRevision: 0, compactRevision: 0}

	opts := c.buildRetryWatchOptions(nil, state)

	// startRev = 0 + 1 = 1，不大于 1，不追加 WithRevision
	if len(opts) != 0 {
		t.Errorf("buildRetryWatchOptions() should not add WithRevision for new watch, got %d opts", len(opts))
	}
}

// TestBuildRetryWatchOptions_PreservesExistingOpts 测试已有选项被保留。
func TestBuildRetryWatchOptions_PreservesExistingOpts(t *testing.T) {
	c := &Client{}
	state := &watchRetryState{lastRevision: 100}

	existingOpts := []WatchOption{WithPrefix()}
	opts := c.buildRetryWatchOptions(existingOpts, state)

	// 应保留 WithPrefix 并追加 WithRevision，共 2 个
	if len(opts) != 2 {
		t.Errorf("buildRetryWatchOptions() should preserve existing opts, got %d", len(opts))
	}
}

// TestHandleWatchRetry_MaxRetriesReached 测试达到最大重试次数时返回 true。
func TestHandleWatchRetry_MaxRetriesReached(t *testing.T) {
	c := &Client{}
	cfg := RetryConfig{
		MaxRetries:        2,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	state := &watchRetryState{
		retryCount: 2,
		backoff:    1 * time.Millisecond,
	}

	shouldStop := c.handleWatchRetry(context.Background(), cfg, state, errors.New("test"))

	if !shouldStop {
		t.Error("handleWatchRetry() should return true when max retries reached")
	}
}

// TestHandleWatchRetry_ContinueRetry 测试未达到最大重试次数时继续重试。
func TestHandleWatchRetry_ContinueRetry(t *testing.T) {
	c := &Client{}
	cfg := RetryConfig{
		MaxRetries:        10,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	state := &watchRetryState{
		retryCount: 0,
		backoff:    1 * time.Millisecond,
	}

	shouldStop := c.handleWatchRetry(context.Background(), cfg, state, errors.New("test"))

	if shouldStop {
		t.Error("handleWatchRetry() should return false when retries remaining")
	}
	if state.retryCount != 1 {
		t.Errorf("retryCount = %d, want 1", state.retryCount)
	}
}

// TestHandleWatchRetry_UnlimitedRetries 测试 MaxRetries=0 时永不停止重试。
func TestHandleWatchRetry_UnlimitedRetries(t *testing.T) {
	c := &Client{}
	cfg := RetryConfig{
		MaxRetries:        0, // 无限重试
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	state := &watchRetryState{
		retryCount: 100,
		backoff:    1 * time.Millisecond,
	}

	shouldStop := c.handleWatchRetry(context.Background(), cfg, state, errors.New("test"))

	if shouldStop {
		t.Error("handleWatchRetry() should never stop with MaxRetries=0")
	}
}

// TestHandleWatchRetry_CallbackInvoked 测试重试时 OnRetry 回调被调用且参数正确。
func TestHandleWatchRetry_CallbackInvoked(t *testing.T) {
	c := &Client{}
	var callbackCalled bool
	var callbackAttempt int
	var callbackErr error

	cfg := RetryConfig{
		MaxRetries:        10,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		OnRetry: func(attempt int, err error, nb time.Duration, lastRevision int64) {
			callbackCalled = true
			callbackAttempt = attempt
			callbackErr = err
		},
	}
	state := &watchRetryState{
		retryCount: 0,
		backoff:    1 * time.Millisecond,
	}

	testErr := errors.New("test error")
	c.handleWatchRetry(context.Background(), cfg, state, testErr)

	if !callbackCalled {
		t.Error("OnRetry callback was not called")
	}
	if callbackAttempt != 1 {
		t.Errorf("callback attempt = %d, want 1", callbackAttempt)
	}
	if callbackErr != testErr {
		t.Errorf("callback error = %v, want %v", callbackErr, testErr)
	}
}

// TestConsumeEventsUntilError_NormalEvents 测试正常事件被转发且通道关闭后返回 false。
func TestConsumeEventsUntilError_NormalEvents(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	innerCh := make(chan Event, 3)
	eventCh := make(chan Event, 10)

	innerCh <- Event{Key: "k1", Revision: 1}
	innerCh <- Event{Key: "k2", Revision: 2}
	innerCh <- Event{Key: "k3", Revision: 3}
	close(innerCh)

	shouldExit, lastRev, compactRev, disconnectErr, consumed := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if shouldExit {
		t.Error("should not exit on normal channel close")
	}
	if lastRev != 3 {
		t.Errorf("lastRevision = %d, want 3", lastRev)
	}
	if compactRev != 0 {
		t.Errorf("compactRevision = %d, want 0", compactRev)
	}
	if disconnectErr != nil {
		t.Errorf("disconnectErr = %v, want nil", disconnectErr)
	}
	if !consumed {
		t.Error("eventsConsumed = false, want true")
	}
	if len(eventCh) != 3 {
		t.Errorf("eventCh has %d events, want 3", len(eventCh))
	}
}

// TestConsumeEventsUntilError_ErrorEvent 测试收到错误事件时返回 false 并记录 compactRevision。
func TestConsumeEventsUntilError_ErrorEvent(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	innerCh := make(chan Event, 3)
	eventCh := make(chan Event, 10)

	innerCh <- Event{Key: "k1", Revision: 5}
	innerCh <- Event{Error: errors.New("watch failed"), Revision: 5, CompactRevision: 100}

	shouldExit, lastRev, compactRev, disconnectErr, consumed := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if shouldExit {
		t.Error("should not exit on error (needs reconnect)")
	}
	if lastRev != 5 {
		t.Errorf("lastRevision = %d, want 5", lastRev)
	}
	if compactRev != 100 {
		t.Errorf("compactRevision = %d, want 100", compactRev)
	}
	if disconnectErr == nil {
		t.Error("disconnectErr should not be nil for error event")
	}
	if !consumed {
		t.Error("eventsConsumed = false, want true (k1 was consumed before error)")
	}
}

// TestConsumeEventsUntilError_ContextCancel 测试 context 取消时返回 true。
func TestConsumeEventsUntilError_ContextCancel(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	innerCh := make(chan Event) // 无缓冲，会阻塞
	eventCh := make(chan Event, 10)

	shouldExit, _, _, disconnectErr, _ := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if !shouldExit {
		t.Error("should exit on context cancel")
	}
	if disconnectErr != nil {
		t.Errorf("disconnectErr = %v, want nil on context cancel", disconnectErr)
	}
}

// TestConsumeEventsUntilError_OutputBlocked 测试输出通道阻塞且 context 取消时返回 true。
func TestConsumeEventsUntilError_OutputBlocked(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithCancel(context.Background())

	innerCh := make(chan Event, 1)
	eventCh := make(chan Event) // 无缓冲，发送会阻塞

	innerCh <- Event{Key: "k1", Revision: 1}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	shouldExit, _, _, disconnectErr, _ := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if !shouldExit {
		t.Error("should exit when output blocked and context canceled")
	}
	if disconnectErr != nil {
		t.Errorf("disconnectErr = %v, want nil on output blocked", disconnectErr)
	}
}

// TestConsumeEventsUntilError_ErrorWithZeroRevision 测试首次失败（revision=0）时正确处理。
func TestConsumeEventsUntilError_ErrorWithZeroRevision(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	innerCh := make(chan Event, 1)
	eventCh := make(chan Event, 10)

	// 未处理任何事件就失败，revision=0
	innerCh <- Event{Error: errors.New("immediate failure"), Revision: 0}

	shouldExit, lastRev, _, disconnectErr, consumed := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if shouldExit {
		t.Error("should not exit on error (needs reconnect)")
	}
	if lastRev != 0 {
		t.Errorf("lastRevision = %d, want 0", lastRev)
	}
	if disconnectErr == nil {
		t.Error("disconnectErr should not be nil for error event")
	}
	if consumed {
		t.Error("eventsConsumed = true, want false (failed before any normal event)")
	}
}

// TestConsumeEventsUntilError_ClientClosed 测试客户端关闭时返回 true。
func TestConsumeEventsUntilError_ClientClosed(t *testing.T) {
	c := &Client{closeCh: make(chan struct{})}
	ctx := context.Background()

	innerCh := make(chan Event) // 无缓冲，会阻塞
	eventCh := make(chan Event, 10)

	// 关闭客户端
	close(c.closeCh)

	shouldExit, _, _, disconnectErr, _ := c.consumeEventsUntilError(ctx, innerCh, eventCh)

	if !shouldExit {
		t.Error("should exit on client close")
	}
	if disconnectErr != nil {
		t.Errorf("disconnectErr = %v, want nil on client close", disconnectErr)
	}
}

// TestDisconnectErrOrDefault 测试 disconnectErrOrDefault 函数。
func TestDisconnectErrOrDefault(t *testing.T) {
	t.Run("nil error returns sentinel", func(t *testing.T) {
		err := disconnectErrOrDefault(nil)
		if !errors.Is(err, ErrWatchDisconnected) {
			t.Errorf("disconnectErrOrDefault(nil) = %v, want ErrWatchDisconnected", err)
		}
	})

	t.Run("non-nil error returns original", func(t *testing.T) {
		original := errors.New("custom error")
		err := disconnectErrOrDefault(original)
		if err != original {
			t.Errorf("disconnectErrOrDefault(err) = %v, want %v", err, original)
		}
	})
}

// TestUpdateAfterConsume 测试 updateAfterConsume 各分支。
func TestUpdateAfterConsume(t *testing.T) {
	t.Run("updates revision", func(t *testing.T) {
		s := &watchRetryState{}
		s.updateAfterConsume(100, 0, true, time.Second)
		if s.lastRevision != 100 {
			t.Errorf("lastRevision = %d, want 100", s.lastRevision)
		}
	})

	t.Run("updates compact revision", func(t *testing.T) {
		s := &watchRetryState{}
		s.updateAfterConsume(0, 200, false, time.Second)
		if s.compactRevision != 200 {
			t.Errorf("compactRevision = %d, want 200", s.compactRevision)
		}
	})

	t.Run("resets retry on consumed", func(t *testing.T) {
		s := &watchRetryState{retryCount: 5, backoff: 10 * time.Second}
		s.updateAfterConsume(100, 0, true, time.Second)
		if s.retryCount != 0 {
			t.Errorf("retryCount = %d, want 0", s.retryCount)
		}
		if s.backoff != time.Second {
			t.Errorf("backoff = %v, want %v", s.backoff, time.Second)
		}
	})

	t.Run("no reset when not consumed", func(t *testing.T) {
		s := &watchRetryState{retryCount: 5, backoff: 10 * time.Second}
		s.updateAfterConsume(0, 0, false, time.Second)
		if s.retryCount != 5 {
			t.Errorf("retryCount = %d, want 5", s.retryCount)
		}
	})

	t.Run("zero revision not updated", func(t *testing.T) {
		s := &watchRetryState{lastRevision: 50}
		s.updateAfterConsume(0, 0, false, time.Second)
		if s.lastRevision != 50 {
			t.Errorf("lastRevision = %d, want 50 (unchanged)", s.lastRevision)
		}
	})

	t.Run("zero compact revision not updated", func(t *testing.T) {
		s := &watchRetryState{compactRevision: 100}
		s.updateAfterConsume(0, 0, false, time.Second)
		if s.compactRevision != 100 {
			t.Errorf("compactRevision = %d, want 100 (unchanged)", s.compactRevision)
		}
	})
}

// TestHandleWatchRetry_ContextCanceled 测试 context 取消时返回 true。
func TestHandleWatchRetry_ContextCanceled(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := RetryConfig{
		MaxRetries:        10,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	state := &watchRetryState{
		retryCount: 0,
		backoff:    1 * time.Millisecond,
	}

	shouldStop := c.handleWatchRetry(ctx, cfg, state, errors.New("test"))
	if !shouldStop {
		t.Error("handleWatchRetry() should return true when context is canceled")
	}
}

// TestForwardEvent 测试 forwardEvent 函数。
func TestForwardEvent(t *testing.T) {
	t.Run("successful forward", func(t *testing.T) {
		c := &Client{closeCh: make(chan struct{})}
		ctx := context.Background()
		eventCh := make(chan Event, 1)

		ok := c.forwardEvent(ctx, eventCh, Event{Key: "k1"})
		if !ok {
			t.Error("forwardEvent() should return true on success")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		c := &Client{closeCh: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		eventCh := make(chan Event) // 无缓冲，会阻塞

		ok := c.forwardEvent(ctx, eventCh, Event{Key: "k1"})
		if ok {
			t.Error("forwardEvent() should return false on context cancel")
		}
	})

	t.Run("client closed", func(t *testing.T) {
		c := &Client{closeCh: make(chan struct{})}
		ctx := context.Background()
		eventCh := make(chan Event) // 无缓冲，会阻塞

		close(c.closeCh)
		ok := c.forwardEvent(ctx, eventCh, Event{Key: "k1"})
		if ok {
			t.Error("forwardEvent() should return false on client close")
		}
	})
}
