package xetcd

import (
	"context"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// noopEtcdClient 最小化的 etcdClient 实现，仅用于前置条件测试。
// 所有方法 panic，因为测试不应该走到实际调用路径。
type noopEtcdClient struct{}

func (n *noopEtcdClient) Get(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	panic("noopEtcdClient.Get should not be called")
}
func (n *noopEtcdClient) Put(context.Context, string, string, ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	panic("noopEtcdClient.Put should not be called")
}
func (n *noopEtcdClient) Delete(context.Context, string, ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	panic("noopEtcdClient.Delete should not be called")
}
func (n *noopEtcdClient) Grant(context.Context, int64) (*clientv3.LeaseGrantResponse, error) {
	panic("noopEtcdClient.Grant should not be called")
}
func (n *noopEtcdClient) Revoke(context.Context, clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	panic("noopEtcdClient.Revoke should not be called")
}
func (n *noopEtcdClient) Watch(context.Context, string, ...clientv3.OpOption) clientv3.WatchChan {
	panic("noopEtcdClient.Watch should not be called")
}
func (n *noopEtcdClient) Close() error {
	panic("noopEtcdClient.Close should not be called")
}

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
