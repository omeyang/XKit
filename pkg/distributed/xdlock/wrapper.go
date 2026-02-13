package xdlock

import (
	"github.com/go-redsync/redsync/v4"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// =============================================================================
// etcd 类型别名
// =============================================================================

// Session 是 etcd concurrency.Session 的类型别名。
// 用于 EtcdFactory.Session() 方法的返回类型。
//
// Session 提供：
//   - 自动续期的 Lease
//   - 基于 Lease 的分布式锁
//   - 基于 Lease 的选举
type Session = *concurrency.Session

// =============================================================================
// Redis (redsync) 类型别名
// =============================================================================

// Redsync 是 redsync.Redsync 的类型别名。
// 用于 RedisFactory.Redsync() 方法的返回类型。
//
// Redsync 提供 Redlock 算法支持（多节点模式）。
type Redsync = *redsync.Redsync
