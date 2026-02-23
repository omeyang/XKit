package xetcd

import (
	"context"
	"testing"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventPut, "PUT"},
		{EventDelete, "DELETE"},
		{EventType(99), "UNKNOWN(99)"},
		{EventUnknown, "UNKNOWN(-1)"},
		{EventType(100), "UNKNOWN(100)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.eventType.String(); got != tt.want {
				t.Errorf("EventType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertEvent_Put(t *testing.T) {
	etcdEvent := &clientv3.Event{
		Type: mvccpb.PUT,
		Kv: &mvccpb.KeyValue{
			Key:         []byte("test-key"),
			Value:       []byte("test-value"),
			ModRevision: 123,
		},
	}

	event := convertEvent(etcdEvent)

	if event.Type != EventPut {
		t.Errorf("convertEvent() Type = %v, want %v", event.Type, EventPut)
	}
	if event.Key != "test-key" {
		t.Errorf("convertEvent() Key = %v, want %v", event.Key, "test-key")
	}
	if string(event.Value) != "test-value" {
		t.Errorf("convertEvent() Value = %v, want %v", string(event.Value), "test-value")
	}
	if event.Revision != 123 {
		t.Errorf("convertEvent() Revision = %v, want %v", event.Revision, 123)
	}
}

func TestConvertEvent_Delete(t *testing.T) {
	etcdEvent := &clientv3.Event{
		Type: mvccpb.DELETE,
		Kv: &mvccpb.KeyValue{
			Key:         []byte("deleted-key"),
			Value:       []byte("should-be-nil"),
			ModRevision: 456,
		},
	}

	event := convertEvent(etcdEvent)

	if event.Type != EventDelete {
		t.Errorf("convertEvent() Type = %v, want %v", event.Type, EventDelete)
	}
	if event.Key != "deleted-key" {
		t.Errorf("convertEvent() Key = %v, want %v", event.Key, "deleted-key")
	}
	if event.Value != nil {
		t.Errorf("convertEvent() Value = %v, want nil for DELETE event", event.Value)
	}
	if event.Revision != 456 {
		t.Errorf("convertEvent() Revision = %v, want %v", event.Revision, 456)
	}
}

func TestConvertEvent_UnknownType(t *testing.T) {
	// 测试未知事件类型，应标记为 EventUnknown
	etcdEvent := &clientv3.Event{
		Type: mvccpb.Event_EventType(99), // 未知类型
		Kv: &mvccpb.KeyValue{
			Key:         []byte("unknown-key"),
			Value:       []byte("unknown-value"),
			ModRevision: 789,
		},
	}

	event := convertEvent(etcdEvent)

	if event.Type != EventUnknown {
		t.Errorf("convertEvent() Type = %v, want EventUnknown", event.Type)
	}
	if event.Key != "unknown-key" {
		t.Errorf("convertEvent() Key = %v, want %v", event.Key, "unknown-key")
	}
	if event.Revision != 789 {
		t.Errorf("convertEvent() Revision = %v, want %v", event.Revision, 789)
	}
}

func TestBuildWatchOptions_Empty(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{})

	if len(opts) != 0 {
		t.Errorf("buildWatchOptions() with empty options should return empty slice, got %d options", len(opts))
	}
}

func TestBuildWatchOptions_WithPrefix(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{prefix: true})

	if len(opts) != 1 {
		t.Errorf("buildWatchOptions() with prefix should return 1 option, got %d", len(opts))
	}
}

func TestBuildWatchOptions_WithRevision(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{revision: 100})

	if len(opts) != 1 {
		t.Errorf("buildWatchOptions() with revision should return 1 option, got %d", len(opts))
	}
}

func TestBuildWatchOptions_WithPrefixAndRevision(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{prefix: true, revision: 100})

	if len(opts) != 2 {
		t.Errorf("buildWatchOptions() with prefix and revision should return 2 options, got %d", len(opts))
	}
}

func TestBuildWatchOptions_ZeroRevision(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{revision: 0})

	// revision <= 0 不应该添加选项
	if len(opts) != 0 {
		t.Errorf("buildWatchOptions() with zero revision should return 0 options, got %d", len(opts))
	}
}

func TestBuildWatchOptions_NegativeRevision(t *testing.T) {
	c := &Client{}
	opts := c.buildWatchOptions(&watchOptions{revision: -1})

	// 负数 revision 不应该添加选项
	if len(opts) != 0 {
		t.Errorf("buildWatchOptions() with negative revision should return 0 options, got %d", len(opts))
	}
}

func TestWithPrefix(t *testing.T) {
	o := &watchOptions{}
	WithPrefix()(o)

	if !o.prefix {
		t.Error("WithPrefix() should set prefix to true")
	}
}

func TestWithRevision(t *testing.T) {
	o := &watchOptions{}
	WithRevision(100)(o)

	if o.revision != 100 {
		t.Errorf("WithRevision() revision = %v, want %v", o.revision, 100)
	}
}

// TestWithBufferSize 测试 WithBufferSize 选项函数。
func TestWithBufferSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		wantSize int
	}{
		{"positive size", 512, 512},
		{"zero size keeps default", 0, 0},
		{"negative size keeps default", -1, 0},
		{"large size", 10000, 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &watchOptions{}
			WithBufferSize(tt.size)(o)
			if o.bufferSize != tt.wantSize {
				t.Errorf("bufferSize = %d, want %d", o.bufferSize, tt.wantSize)
			}
		})
	}
}

func TestWatch_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.Watch(context.Background(), "key")
	if err != ErrClientClosed {
		t.Errorf("Watch() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestWatch_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.Watch(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("Watch() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestDispatchEvents_Success(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	eventCh := make(chan Event, 10)

	events := []*clientv3.Event{
		createPutEvent("key1", "value1", 1),
		createPutEvent("key2", "value2", 2),
	}

	_, result := c.dispatchEvents(ctx, events, eventCh)

	if !result {
		t.Error("dispatchEvents() should return true on success")
	}

	// 验证事件被正确发送
	if len(eventCh) != 2 {
		t.Errorf("dispatchEvents() sent %d events, want 2", len(eventCh))
	}

	event1 := <-eventCh
	if event1.Key != "key1" || string(event1.Value) != "value1" {
		t.Errorf("dispatchEvents() event1 = {%s, %s}, want {key1, value1}", event1.Key, event1.Value)
	}

	event2 := <-eventCh
	if event2.Key != "key2" || string(event2.Value) != "value2" {
		t.Errorf("dispatchEvents() event2 = {%s, %s}, want {key2, value2}", event2.Key, event2.Value)
	}
}

func TestDispatchEvents_ContextCanceled(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	eventCh := make(chan Event) // 无缓冲，会阻塞

	events := []*clientv3.Event{
		createPutEvent("key1", "value1", 1),
	}

	_, result := c.dispatchEvents(ctx, events, eventCh)

	if result {
		t.Error("dispatchEvents() should return false when context is canceled")
	}
}

func TestDispatchEvents_ClientClosed(t *testing.T) {
	c := &Client{closeCh: make(chan struct{})}
	ctx := context.Background()

	eventCh := make(chan Event) // 无缓冲，发送会阻塞

	// 关闭 closeCh 触发退出
	close(c.closeCh)

	events := []*clientv3.Event{
		createPutEvent("key1", "value1", 1),
	}

	_, result := c.dispatchEvents(ctx, events, eventCh)

	if result {
		t.Error("dispatchEvents() should return false when client is closed")
	}
}

func TestDispatchEvents_EmptyEvents(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	eventCh := make(chan Event, 10)

	_, result := c.dispatchEvents(ctx, nil, eventCh)

	if !result {
		t.Error("dispatchEvents() should return true for empty events")
	}

	if len(eventCh) != 0 {
		t.Errorf("dispatchEvents() sent %d events for empty input, want 0", len(eventCh))
	}
}

func TestDispatchEvents_MixedEventTypes(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	eventCh := make(chan Event, 10)

	events := []*clientv3.Event{
		createPutEvent("put-key", "put-value", 1),
		createDeleteEvent("delete-key", 2),
	}

	_, result := c.dispatchEvents(ctx, events, eventCh)

	if !result {
		t.Error("dispatchEvents() should return true on success")
	}

	putEvent := <-eventCh
	if putEvent.Type != EventPut {
		t.Errorf("dispatchEvents() first event type = %v, want EventPut", putEvent.Type)
	}

	deleteEvent := <-eventCh
	if deleteEvent.Type != EventDelete {
		t.Errorf("dispatchEvents() second event type = %v, want EventDelete", deleteEvent.Type)
	}
}
