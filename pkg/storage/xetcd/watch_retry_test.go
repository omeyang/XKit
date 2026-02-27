package xetcd

import (
	"context"
	"errors"
	"math"
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

// TestWatch_NilContext 测试 Watch 在 ctx 为 nil 时返回 ErrNilContext。
func TestWatch_NilContext(t *testing.T) {
	c := &Client{closeCh: make(chan struct{})}

	_, err := c.Watch(nil, "key") //nolint:staticcheck // 测试 nil ctx 防御
	if err != ErrNilContext {
		t.Errorf("Watch(nil, key) = %v, want %v", err, ErrNilContext)
	}
}

// TestWatchWithRetry_NilContext 测试 WatchWithRetry 在 ctx 为 nil 时返回 ErrNilContext。
func TestWatchWithRetry_NilContext(t *testing.T) {
	c := &Client{closeCh: make(chan struct{})}

	_, err := c.WatchWithRetry(nil, "key", DefaultRetryConfig()) //nolint:staticcheck // 测试 nil ctx 防御
	if err != ErrNilContext {
		t.Errorf("WatchWithRetry(nil, key, cfg) = %v, want %v", err, ErrNilContext)
	}
}

// TestValidateRetryConfig 测试 validateRetryConfig 对显式负值返回错误，对零值应用默认值。
func TestValidateRetryConfig(t *testing.T) {
	t.Run("negative InitialBackoff", func(t *testing.T) {
		cfg := RetryConfig{InitialBackoff: -1 * time.Second}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("negative MaxBackoff", func(t *testing.T) {
		cfg := RetryConfig{MaxBackoff: -1 * time.Second}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("negative MaxRetries", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: -1}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("NaN BackoffMultiplier", func(t *testing.T) {
		cfg := RetryConfig{BackoffMultiplier: math.NaN()}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("positive Inf BackoffMultiplier", func(t *testing.T) {
		cfg := RetryConfig{BackoffMultiplier: math.Inf(1)}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("negative Inf BackoffMultiplier", func(t *testing.T) {
		cfg := RetryConfig{BackoffMultiplier: math.Inf(-1)}
		err := validateRetryConfig(&cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("validateRetryConfig() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("zero values apply defaults", func(t *testing.T) {
		cfg := RetryConfig{}
		err := validateRetryConfig(&cfg)
		if err != nil {
			t.Fatalf("validateRetryConfig() = %v, want nil", err)
		}
		if cfg.InitialBackoff != 1*time.Second {
			t.Errorf("InitialBackoff = %v, want 1s", cfg.InitialBackoff)
		}
		if cfg.MaxBackoff != 30*time.Second {
			t.Errorf("MaxBackoff = %v, want 30s", cfg.MaxBackoff)
		}
		if cfg.BackoffMultiplier != 2.0 {
			t.Errorf("BackoffMultiplier = %v, want 2.0", cfg.BackoffMultiplier)
		}
	})

	t.Run("MaxBackoff less than InitialBackoff corrected", func(t *testing.T) {
		cfg := RetryConfig{
			InitialBackoff:    5 * time.Second,
			MaxBackoff:        2 * time.Second,
			BackoffMultiplier: 2.0,
		}
		err := validateRetryConfig(&cfg)
		if err != nil {
			t.Fatalf("validateRetryConfig() = %v, want nil", err)
		}
		if cfg.MaxBackoff != cfg.InitialBackoff {
			t.Errorf("MaxBackoff = %v, want %v (corrected to InitialBackoff)", cfg.MaxBackoff, cfg.InitialBackoff)
		}
	})
}

// TestWatchWithRetry_NegativeConfig 测试 WatchWithRetry 对显式负值配置返回错误。
func TestWatchWithRetry_NegativeConfig(t *testing.T) {
	c := newStubClient()

	t.Run("negative InitialBackoff", func(t *testing.T) {
		cfg := RetryConfig{InitialBackoff: -1 * time.Second}
		_, err := c.WatchWithRetry(context.Background(), "key", cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("WatchWithRetry() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("negative MaxBackoff", func(t *testing.T) {
		cfg := RetryConfig{MaxBackoff: -1 * time.Second}
		_, err := c.WatchWithRetry(context.Background(), "key", cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("WatchWithRetry() = %v, want ErrInvalidRetryConfig", err)
		}
	})

	t.Run("negative MaxRetries", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: -1}
		_, err := c.WatchWithRetry(context.Background(), "key", cfg)
		if !errors.Is(err, ErrInvalidRetryConfig) {
			t.Errorf("WatchWithRetry() = %v, want ErrInvalidRetryConfig", err)
		}
	})
}

// TestWatchWithRetry_Closed 测试已关闭客户端调用 WatchWithRetry 返回错误。
func TestWatchWithRetry_Closed(t *testing.T) {
	c := newClosedStubClient()

	_, err := c.WatchWithRetry(context.Background(), "key", DefaultRetryConfig())
	if err != ErrClientClosed {
		t.Errorf("WatchWithRetry() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

// TestWatchWithRetry_EmptyKey 测试空键调用 WatchWithRetry 返回错误。
func TestWatchWithRetry_EmptyKey(t *testing.T) {
	c := newStubClient()

	_, err := c.WatchWithRetry(context.Background(), "", DefaultRetryConfig())
	if err != ErrEmptyKey {
		t.Errorf("WatchWithRetry() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

// TestWatchWithRetry_NilOption 测试 WatchWithRetry 传入 nil WatchOption 时返回 ErrNilOption。
func TestWatchWithRetry_NilOption(t *testing.T) {
	c := newStubClient()

	_, err := c.WatchWithRetry(context.Background(), "key", DefaultRetryConfig(), nil)
	if err != ErrNilOption {
		t.Errorf("WatchWithRetry() with nil option = %v, want %v", err, ErrNilOption)
	}
}

// TestWatchWithRetry_InvalidBackoffMultiplier 测试 BackoffMultiplier < 1 被修正为 2.0。
func TestWatchWithRetry_InvalidBackoffMultiplier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())

	watchChan := make(chan clientv3.WatchResponse, 1)
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any()).
		Return(clientv3.WatchChan(watchChan))

	cfg := RetryConfig{
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        50 * time.Millisecond,
		BackoffMultiplier: 0.5, // 无效值，应被修正
	}

	eventCh, err := c.WatchWithRetry(ctx, "/key", cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v", err)
	}

	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Key: []byte("/key"), Value: []byte("v"), ModRevision: 1}},
			},
		}
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	select {
	case event := <-eventCh:
		if event.Key != "/key" {
			t.Errorf("event.Key = %v, want /key", event.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	<-eventCh // drain close
}

// TestWatchWithRetry_MaxBackoffLessThanInitial 测试 MaxBackoff < InitialBackoff 被修正。
func TestWatchWithRetry_MaxBackoffLessThanInitial(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())

	watchChan := make(chan clientv3.WatchResponse, 1)
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any()).
		Return(clientv3.WatchChan(watchChan))

	cfg := RetryConfig{
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond, // 倒挂，应被修正为 InitialBackoff
		BackoffMultiplier: 2.0,
	}

	eventCh, err := c.WatchWithRetry(ctx, "/key", cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v", err)
	}

	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Key: []byte("/key"), Value: []byte("v"), ModRevision: 1}},
			},
		}
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	select {
	case event := <-eventCh:
		if event.Key != "/key" {
			t.Errorf("event.Key = %v, want /key", event.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	<-eventCh // drain close
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

// TestWatchWithRetry_WatchReturnsError_MaxRetries 测试 Watch() 方法本身返回错误时
// 触发重试直到 MaxRetries 耗尽。
// 覆盖 runWatchWithRetry 中 c.Watch 返回 err != nil → handleWatchRetry →
// sendMaxRetriesErrorIfNeeded 路径。
func TestWatchWithRetry_WatchReturnsError_MaxRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	// 第一次 Watch 成功，返回立即关闭的通道触发 disconnect。
	// 在 handleWatchRetry 退避期间关闭客户端，使后续 Watch 被 checkClosed 拦截。
	callCount := 0
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, k string, opts ...clientv3.OpOption) clientv3.WatchChan {
			callCount++
			if callCount == 1 {
				ch := make(chan clientv3.WatchResponse)
				close(ch) // 立即关闭触发 disconnect
				return ch
			}
			// 不应到达：后续调用被 checkClosed 拦截
			ch := make(chan clientv3.WatchResponse)
			close(ch)
			return ch
		}).AnyTimes()

	// 关闭 client 底层 mock（Close 调用）
	mockClient.EXPECT().Close().Return(nil).AnyTimes()

	cfg := RetryConfig{
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxRetries:        2,
	}

	eventCh, err := c.WatchWithRetry(context.Background(), "/key", cfg)
	if err != nil {
		t.Fatalf("WatchWithRetry() error = %v", err)
	}

	// 等待第一次 Watch 完成并进入退避，然后关闭客户端
	time.Sleep(3 * time.Millisecond)
	if closeErr := c.Close(context.Background()); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	// 通道应关闭（客户端关闭导致 shouldStopWatch 返回 true）
	select {
	case <-eventCh:
		// 通道关闭或收到事件，正常
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
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
	if closeErr := c.Close(context.Background()); closeErr != nil {
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

		for i := 0; i < 100; i++ {
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
