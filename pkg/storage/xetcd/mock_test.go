package xetcd

import (
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// createTestEvent 创建测试用的 etcd 事件。
func createTestEvent(eventType mvccpb.Event_EventType, key, value string, revision int64) *clientv3.Event {
	return &clientv3.Event{
		Type: eventType,
		Kv: &mvccpb.KeyValue{
			Key:         []byte(key),
			Value:       []byte(value),
			ModRevision: revision,
		},
	}
}

// createPutEvent 创建 PUT 事件。
func createPutEvent(key, value string, revision int64) *clientv3.Event {
	return createTestEvent(mvccpb.PUT, key, value, revision)
}

// createDeleteEvent 创建 DELETE 事件。
func createDeleteEvent(key string, revision int64) *clientv3.Event {
	return &clientv3.Event{
		Type: mvccpb.DELETE,
		Kv: &mvccpb.KeyValue{
			Key:         []byte(key),
			ModRevision: revision,
		},
	}
}
