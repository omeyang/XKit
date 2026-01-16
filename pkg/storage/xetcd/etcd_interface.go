package xetcd

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdKV 定义 etcd KV 操作接口，用于依赖注入和测试。
// 接口方法与 clientv3.KV 保持一致。
type etcdKV interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
}

// etcdLease 定义 etcd 租约操作接口，用于依赖注入和测试。
// 接口方法与 clientv3.Lease 保持一致。
type etcdLease interface {
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
}

// etcdWatcher 定义 etcd Watch 操作接口，用于依赖注入和测试。
// 接口方法与 clientv3.Watcher 保持一致。
type etcdWatcher interface {
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
}

// etcdClient 组合接口，包含所有 xetcd 需要的 etcd 操作。
// *clientv3.Client 实现了此接口。
type etcdClient interface {
	etcdKV
	etcdLease
	etcdWatcher
	Close() error
}

// 确保 *clientv3.Client 实现 etcdClient 接口（编译时检查）
var _ etcdClient = (*clientv3.Client)(nil)
