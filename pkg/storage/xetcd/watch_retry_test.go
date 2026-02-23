package xetcd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"
)

// TestDefaultRetryConfig 验证默认重试配置的字段值。
func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.InitialBackoff != 1*time.Second {
		t.Errorf("InitialBackoff = %v, want %v", cfg.InitialBackoff, 1*time.Second)
	}
	if cfg.MaxBackoff != 30*time.Second {
		t.Errorf("MaxBackoff = %v, want %v", cfg.MaxBackoff, 30*time.Second)
	}
	if cfg.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %v, want %v", cfg.BackoffMultiplier, 2.0)
	}
	if cfg.MaxRetries != 0 {
		t.Errorf("MaxRetries = %v, want %v", cfg.MaxRetries, 0)
	}
	if cfg.OnRetry != nil {
		t.Error("OnRetry should be nil by default")
	}
}

// TestWatchWithRetry_Closed 测试已关闭客户端调用 WatchWithRetry 返回错误。
func TestWatchWithRetry_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.WatchWithRetry(context.Background(), "key", DefaultRetryConfig())
	if err != ErrClientClosed {
		t.Errorf("WatchWithRetry() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

// TestWatchWithRetry_EmptyKey 测试空键调用 WatchWithRetry 返回错误。
func TestWatchWithRetry_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.WatchWithRetry(context.Background(), "", DefaultRetryConfig())
	if err != ErrEmptyKey {
		t.Errorf("WatchWithRetry() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

// TestWatchWithRetry_DefaultValues 测试零值配置触发默认值补充后正常运行。
func TestWatchWithRetry_DefaultValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse, 1)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	// 使用零值触发默认值
	cfg := RetryConfig{}

	eventCh, err := c.WatchWithRetry(ctx, key, cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v, want nil", err)
	}

	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.PUT,
					Kv: &mvccpb.KeyValue{
						Key:         []byte(key),
						Value:       []byte("value"),
						ModRevision: 100,
					},
				},
			},
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("eventCh closed unexpectedly")
		}
		if event.Key != key {
			t.Errorf("event.Key = %v, want %v", event.Key, key)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// 等待通道关闭
	select {
	case <-eventCh:
	case <-time.After(time.Second):
	}
}

// TestWatchWithRetry_ReconnectOnError 测试 Watch 失败后自动重连并接收新事件。
func TestWatchWithRetry_ReconnectOnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"

	watchChan1 := make(chan clientv3.WatchResponse, 1)
	watchChan2 := make(chan clientv3.WatchResponse, 1)

	callCount := 0
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, k string, opts ...clientv3.OpOption) clientv3.WatchChan {
			callCount++
			if callCount == 1 {
				return watchChan1
			}
			return watchChan2
		}).
		Times(2)

	var retryCallbackCalled atomic.Bool
	cfg := RetryConfig{
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        50 * time.Millisecond,
		BackoffMultiplier: 2.0,
		OnRetry: func(attempt int, err error, nb time.Duration, lastRevision int64) {
			retryCallbackCalled.Store(true)
		},
	}

	eventCh, err := c.WatchWithRetry(ctx, key, cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v, want nil", err)
	}

	// 第一个通道发送错误
	go func() {
		watchChan1 <- clientv3.WatchResponse{Canceled: true}
		close(watchChan1)
	}()

	// 第二个通道延迟后发送正常事件，然后触发取消
	go func() {
		time.Sleep(100 * time.Millisecond)
		watchChan2 <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.PUT,
					Kv: &mvccpb.KeyValue{
						Key:         []byte(key),
						Value:       []byte("reconnected"),
						ModRevision: 200,
					},
				},
			},
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("eventCh closed unexpectedly")
		}
		if string(event.Value) != "reconnected" {
			t.Errorf("event.Value = %q, want %q", event.Value, "reconnected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event after reconnect")
	}

	if !retryCallbackCalled.Load() {
		t.Error("OnRetry callback was not called")
	}
}

// TestWatchWithRetry_MaxRetries 测试达到最大重试次数后发送 ErrMaxRetriesExceeded 错误事件，然后关闭通道。
func TestWatchWithRetry_MaxRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "/app/config"

	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, k string, opts ...clientv3.OpOption) clientv3.WatchChan {
			ch := make(chan clientv3.WatchResponse, 1)
			ch <- clientv3.WatchResponse{Canceled: true}
			close(ch)
			return ch
		}).
		AnyTimes()

	cfg := RetryConfig{
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 1.5,
		MaxRetries:        2,
	}

	eventCh, err := c.WatchWithRetry(ctx, key, cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v, want nil", err)
	}

	// 应收到 ErrMaxRetriesExceeded 错误事件
	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("channel closed without error event")
		}
		if !errors.Is(event.Error, ErrMaxRetriesExceeded) {
			t.Errorf("event.Error = %v, want ErrMaxRetriesExceeded", event.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error event after max retries")
	}

	// 随后通道应该关闭
	select {
	case _, ok := <-eventCh:
		if ok {
			t.Error("channel should be closed after error event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

// TestWatchWithRetry_ClientClosed 测试客户端关闭时 WatchWithRetry 退出。
func TestWatchWithRetry_ClientClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	// 关闭客户端底层 mock
	mockClient.EXPECT().Close().Return(nil)

	cfg := RetryConfig{
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	eventCh, err := c.WatchWithRetry(ctx, key, cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v, want nil", err)
	}

	// 关闭客户端，应触发 closeCh 信号
	time.Sleep(10 * time.Millisecond)
	if closeErr := c.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	// 通道应该在客户端关闭后关闭
	select {
	case <-eventCh:
		// 通道已关闭
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for channel close after client close")
	}
}

// TestWatchWithRetry_WatchErrorWithRevision 测试 Watch 错误事件包含 revision 时正确恢复。
func TestWatchWithRetry_WatchErrorWithRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"

	watchChan1 := make(chan clientv3.WatchResponse, 2)
	watchChan2 := make(chan clientv3.WatchResponse, 1)

	callCount := 0
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, k string, opts ...clientv3.OpOption) clientv3.WatchChan {
			callCount++
			if callCount == 1 {
				return watchChan1
			}
			return watchChan2
		}).
		Times(2)

	cfg := RetryConfig{
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	eventCh, err := c.WatchWithRetry(ctx, key, cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v, want nil", err)
	}

	// 先发送正常事件，再发送错误
	watchChan1 <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: mvccpb.PUT,
				Kv:   &mvccpb.KeyValue{Key: []byte(key), Value: []byte("v1"), ModRevision: 50},
			},
		},
	}
	// 关闭第一个通道触发 disconnectErrOrDefault 路径
	close(watchChan1)

	// 读取正常事件
	select {
	case event := <-eventCh:
		if event.Revision != 50 {
			t.Errorf("event.Revision = %d, want 50", event.Revision)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first event")
	}

	// 第二个通道发送事件后取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		watchChan2 <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.PUT,
					Kv:   &mvccpb.KeyValue{Key: []byte(key), Value: []byte("v2"), ModRevision: 60},
				},
			},
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	select {
	case event := <-eventCh:
		if string(event.Value) != "v2" {
			t.Errorf("event.Value = %q, want v2", event.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reconnected event")
	}
}

// TestNextBackoff 测试退避时间计算逻辑。
func TestNextBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{"1s to 2s", 1 * time.Second, 2 * time.Second},
		{"2s to 4s", 2 * time.Second, 4 * time.Second},
		{"15s to 30s", 15 * time.Second, 30 * time.Second},
		{"20s capped at 30s", 20 * time.Second, 30 * time.Second},
		{"30s stays 30s", 30 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBackoff(tt.current, cfg)
			if got != tt.want {
				t.Errorf("nextBackoff(%v) = %v, want %v", tt.current, got, tt.want)
			}
		})
	}
}

// TestSleepWithContext_Normal 测试正常睡眠等待定时器触发。
func TestSleepWithContext_Normal(t *testing.T) {
	start := time.Now()
	sleepWithContext(context.Background(), 10*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 5*time.Millisecond {
		t.Errorf("sleepWithContext() returned too quickly: %v", elapsed)
	}
}

// TestSleepWithContext_ContextCancel 测试 context 取消时立即返回。
func TestSleepWithContext_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	sleepWithContext(ctx, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("sleepWithContext() should return quickly on cancel, took %v", elapsed)
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
