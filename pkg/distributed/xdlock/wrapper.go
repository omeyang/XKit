package xdlock

import (
	"github.com/go-redsync/redsync/v4"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// =============================================================================
// etcd 类型别名
// =============================================================================

// Client 是 etcd clientv3.Client 的类型别名。
// 用于 NewEtcdClient() 方法的返回类型。
//
// Client 提供完整的 etcd 客户端功能：
//   - KV: 键值操作（Get, Put, Delete, Txn）
//   - Lease: 租约操作（Grant, Revoke, KeepAlive）
//   - Watcher: 监听变化
//   - Cluster: 集群管理
//   - Auth: 认证授权
type Client = *clientv3.Client

// Session 是 etcd concurrency.Session 的类型别名。
// 用于 EtcdFactory.Session() 方法的返回类型。
//
// Session 提供：
//   - 自动续期的 Lease
//   - 基于 Lease 的分布式锁
//   - 基于 Lease 的选举
type Session = *concurrency.Session

// Mutex 是 etcd concurrency.Mutex 的类型别名。
//
// Mutex 提供：
//   - Lock: 阻塞式获取锁
//   - TryLock: 非阻塞式获取锁
//   - Unlock: 释放锁
//   - Key: 获取锁的 key
type Mutex = *concurrency.Mutex

// =============================================================================
// Redis (redsync) 类型别名
// =============================================================================

// Redsync 是 redsync.Redsync 的类型别名。
// 用于 RedisFactory.Redsync() 方法的返回类型。
//
// Redsync 提供 Redlock 算法支持（多节点模式）。
type Redsync = *redsync.Redsync

// RedisMutex 是 redsync.Mutex 的类型别名。
//
// RedisMutex 提供：
//   - Lock: 阻塞式获取锁
//   - TryLock: 非阻塞式获取锁
//   - Unlock: 释放锁
//   - Extend: 续期锁
//   - Name: 获取锁的名称
//   - Value: 获取锁的唯一值
type RedisMutex = *redsync.Mutex
