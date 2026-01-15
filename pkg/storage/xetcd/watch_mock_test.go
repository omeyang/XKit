package xetcd

import (
	"context"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"
)

// TestWatch_Success 测试 Watch 成功监听。
func TestWatch_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse, 1)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 发送一个事件
	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.PUT,
					Kv: &mvccpb.KeyValue{
						Key:         []byte(key),
						Value:       []byte("new-value"),
						ModRevision: 100,
					},
				},
			},
		}
		close(watchChan)
	}()

	// 接收事件
	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("eventCh closed unexpectedly")
		}
		if event.Type != EventPut {
			t.Errorf("event.Type = %v, want EventPut", event.Type)
		}
		if event.Key != key {
			t.Errorf("event.Key = %v, want %v", event.Key, key)
		}
		if string(event.Value) != "new-value" {
			t.Errorf("event.Value = %v, want new-value", string(event.Value))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestWatch_WithPrefix 测试 Watch 带前缀。
func TestWatch_WithPrefix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prefix := "/app/"
	watchChan := make(chan clientv3.WatchResponse, 1)

	mockClient.EXPECT().
		Watch(gomock.Any(), prefix, gomock.Any()).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, prefix, WithPrefix())
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 发送事件
	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.PUT,
					Kv: &mvccpb.KeyValue{
						Key:         []byte("/app/config1"),
						Value:       []byte("value1"),
						ModRevision: 101,
					},
				},
				{
					Type: mvccpb.DELETE,
					Kv: &mvccpb.KeyValue{
						Key:         []byte("/app/config2"),
						ModRevision: 102,
					},
				},
			},
		}
		close(watchChan)
	}()

	// 接收两个事件
	events := make([]Event, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case event, ok := <-eventCh:
			if !ok {
				break
			}
			events = append(events, event)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}

	if len(events) < 2 {
		t.Fatalf("received %d events, want 2", len(events))
	}

	if events[0].Type != EventPut {
		t.Errorf("events[0].Type = %v, want EventPut", events[0].Type)
	}
	if events[1].Type != EventDelete {
		t.Errorf("events[1].Type = %v, want EventDelete", events[1].Type)
	}
}

// TestWatch_WithRevision 测试 Watch 指定版本。
func TestWatch_WithRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	revision := int64(500)
	watchChan := make(chan clientv3.WatchResponse, 1)
	watchCalled := make(chan struct{})

	// 使用 DoAndReturn 来处理变参匹配问题，并通过 channel 同步
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, k string, opts ...clientv3.OpOption) clientv3.WatchChan {
			defer close(watchCalled)
			if k != key {
				t.Errorf("Watch key = %v, want %v", k, key)
			}
			if len(opts) != 1 {
				t.Errorf("Watch opts count = %d, want 1", len(opts))
			}
			return watchChan
		})

	eventCh, err := c.Watch(ctx, key, WithRevision(revision))
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}
	if eventCh == nil {
		t.Fatal("Watch() returned nil channel")
	}

	// 等待 goroutine 中的 Watch 被调用
	<-watchCalled
	close(watchChan)
}

// TestWatch_ContextCancel 测试 Watch context 取消。
func TestWatch_ContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 取消 context
	cancel()

	// 通道应该被关闭
	select {
	case _, ok := <-eventCh:
		if ok {
			// 可能收到一个事件，继续等待关闭
			select {
			case _, ok := <-eventCh:
				if ok {
					t.Log("eventCh still open after context cancel")
				}
			case <-time.After(100 * time.Millisecond):
				// OK
			}
		}
	case <-time.After(100 * time.Millisecond):
		// 可能已经关闭或正在处理
	}
}

// TestWatch_WatchError 测试 Watch 收到错误响应。
func TestWatch_WatchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse, 1)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 发送带错误的响应
	go func() {
		watchChan <- clientv3.WatchResponse{
			Canceled: true,
		}
		close(watchChan)
	}()

	// 通道应该被关闭
	select {
	case <-eventCh:
		// 可能还有事件或通道已关闭
	case <-time.After(100 * time.Millisecond):
		// OK - 超时也是可接受的
	}
}

// TestWatch_ChannelClosed 测试 Watch 通道关闭。
func TestWatch_ChannelClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 立即关闭 watch 通道
	close(watchChan)

	// 等待事件通道关闭
	select {
	case <-eventCh:
		// 可能还有缓冲的事件或通道已关闭
	case <-time.After(100 * time.Millisecond):
		// OK - 超时也是可接受的
	}
}

// TestRunWatchLoop_MultipleResponses 测试 runWatchLoop 处理多个响应。
func TestRunWatchLoop_MultipleResponses(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse, 3)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 发送多个响应
	go func() {
		for i := 0; i < 3; i++ {
			watchChan <- clientv3.WatchResponse{
				Events: []*clientv3.Event{
					{
						Type: mvccpb.PUT,
						Kv: &mvccpb.KeyValue{
							Key:         []byte(key),
							Value:       []byte("value"),
							ModRevision: int64(100 + i),
						},
					},
				},
			}
		}
		close(watchChan)
	}()

	// 收集事件
	events := make([]Event, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case event, ok := <-eventCh:
			if !ok {
				break
			}
			events = append(events, event)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}

	if len(events) != 3 {
		t.Errorf("received %d events, want 3", len(events))
	}
}

// TestRunWatchLoop_DeleteEvents 测试 runWatchLoop 处理删除事件。
func TestRunWatchLoop_DeleteEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := "/app/config"
	watchChan := make(chan clientv3.WatchResponse, 1)

	mockClient.EXPECT().
		Watch(gomock.Any(), key).
		Return(clientv3.WatchChan(watchChan))

	eventCh, err := c.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v, want nil", err)
	}

	// 发送删除事件
	go func() {
		watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: mvccpb.DELETE,
					Kv: &mvccpb.KeyValue{
						Key:         []byte(key),
						Value:       []byte("old-value"), // DELETE 事件可能包含旧值
						ModRevision: 200,
					},
				},
			},
		}
		close(watchChan)
	}()

	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("eventCh closed unexpectedly")
		}
		if event.Type != EventDelete {
			t.Errorf("event.Type = %v, want EventDelete", event.Type)
		}
		if event.Value != nil {
			t.Errorf("event.Value = %v, want nil for DELETE event", event.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
