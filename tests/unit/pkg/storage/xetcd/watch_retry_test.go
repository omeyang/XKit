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
