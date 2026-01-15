package xetcd

import (
	"context"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// BenchmarkEventType_String 测试 EventType.String 性能。
func BenchmarkEventType_String(b *testing.B) {
	eventTypes := []EventType{EventPut, EventDelete, EventType(99)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, et := range eventTypes {
			_ = et.String()
		}
	}
}

// BenchmarkConvertEvent_Put 测试 PUT 事件转换性能。
func BenchmarkConvertEvent_Put(b *testing.B) {
	event := &clientv3.Event{
		Type: mvccpb.PUT,
		Kv: &mvccpb.KeyValue{
			Key:         []byte("benchmark-key"),
			Value:       []byte("benchmark-value-with-some-data"),
			ModRevision: 12345,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertEvent(event)
	}
}

// BenchmarkConvertEvent_Delete 测试 DELETE 事件转换性能。
func BenchmarkConvertEvent_Delete(b *testing.B) {
	event := &clientv3.Event{
		Type: mvccpb.DELETE,
		Kv: &mvccpb.KeyValue{
			Key:         []byte("benchmark-deleted-key"),
			ModRevision: 12345,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertEvent(event)
	}
}

// BenchmarkBuildWatchOptions 测试构建 watch 选项性能。
func BenchmarkBuildWatchOptions(b *testing.B) {
	c := &Client{}
	opts := &watchOptions{prefix: true, revision: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.buildWatchOptions(opts)
	}
}

// BenchmarkBuildWatchOptions_Empty 测试空选项构建性能。
func BenchmarkBuildWatchOptions_Empty(b *testing.B) {
	c := &Client{}
	opts := &watchOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.buildWatchOptions(opts)
	}
}

// BenchmarkDispatchEvents 测试事件分发性能。
func BenchmarkDispatchEvents(b *testing.B) {
	c := &Client{}
	ctx := context.Background()
	events := make([]*clientv3.Event, 10)
	for i := range events {
		events[i] = &clientv3.Event{
			Type: mvccpb.PUT,
			Kv: &mvccpb.KeyValue{
				Key:         []byte("key"),
				Value:       []byte("value"),
				ModRevision: int64(i),
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventCh := make(chan Event, len(events))
		c.dispatchEvents(ctx, events, eventCh)
		close(eventCh)
	}
}

// BenchmarkConfig_Validate 测试配置验证性能。
func BenchmarkConfig_Validate(b *testing.B) {
	cfg := &Config{
		Endpoints: []string{"localhost:2379", "localhost:2380", "localhost:2381"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Validate()
	}
}

// BenchmarkConfig_ApplyDefaults 测试应用默认值性能。
func BenchmarkConfig_ApplyDefaults(b *testing.B) {
	cfg := &Config{
		Endpoints: []string{"localhost:2379"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.applyDefaults()
	}
}

// BenchmarkDefaultConfig 测试获取默认配置性能。
func BenchmarkDefaultConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// BenchmarkIsKeyNotFound 测试错误检查性能。
func BenchmarkIsKeyNotFound(b *testing.B) {
	err := ErrKeyNotFound

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsKeyNotFound(err)
	}
}

// BenchmarkIsClientClosed 测试错误检查性能。
func BenchmarkIsClientClosed(b *testing.B) {
	err := ErrClientClosed

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsClientClosed(err)
	}
}

// BenchmarkWithPrefix 测试 WithPrefix 选项性能。
func BenchmarkWithPrefix(b *testing.B) {
	for i := 0; i < b.N; i++ {
		o := &watchOptions{}
		WithPrefix()(o)
	}
}

// BenchmarkWithRevision 测试 WithRevision 选项性能。
func BenchmarkWithRevision(b *testing.B) {
	for i := 0; i < b.N; i++ {
		o := &watchOptions{}
		WithRevision(12345)(o)
	}
}

// BenchmarkWithContext 测试 WithContext 选项性能。
func BenchmarkWithContext(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := defaultOptions()
		WithContext(ctx)(o)
	}
}

// BenchmarkWithHealthCheck 测试 WithHealthCheck 选项性能。
func BenchmarkWithHealthCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		o := defaultOptions()
		WithHealthCheck(true, 5*time.Second)(o)
	}
}

// BenchmarkCheckClosed 测试检查关闭状态性能。
func BenchmarkCheckClosed(b *testing.B) {
	c := &Client{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.checkClosed()
	}
}

// BenchmarkIsClosed 测试获取关闭状态性能。
func BenchmarkIsClosed(b *testing.B) {
	c := &Client{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.isClosed()
	}
}

// BenchmarkDispatchEvents_Large 测试大量事件分发性能。
func BenchmarkDispatchEvents_Large(b *testing.B) {
	c := &Client{}
	ctx := context.Background()
	events := make([]*clientv3.Event, 100)
	for i := range events {
		events[i] = &clientv3.Event{
			Type: mvccpb.PUT,
			Kv: &mvccpb.KeyValue{
				Key:         []byte("key-with-longer-name-for-benchmark"),
				Value:       []byte("value-with-longer-content-for-realistic-benchmark-testing"),
				ModRevision: int64(i),
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventCh := make(chan Event, len(events))
		c.dispatchEvents(ctx, events, eventCh)
		close(eventCh)
	}
}

// BenchmarkConvertEvent_Parallel 并行测试事件转换性能。
func BenchmarkConvertEvent_Parallel(b *testing.B) {
	event := &clientv3.Event{
		Type: mvccpb.PUT,
		Kv: &mvccpb.KeyValue{
			Key:         []byte("parallel-key"),
			Value:       []byte("parallel-value"),
			ModRevision: 12345,
		},
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = convertEvent(event)
		}
	})
}
