// Package xetcd 提供 etcd 客户端封装。
//
// xetcd 是 xkit 存储模块的一部分，提供：
//   - 简化的 KV 操作 (Get/Put/Delete/List/Exists/Count)
//   - PutWithTTL 带租约的键值写入
//   - Watch 功能，监听键值变化
//   - WatchWithRetry 带自动重连和指数退避的 Watch
//   - 与 xdlock 分布式锁的集成
//
// # 设计边界
//
// 设计决策: xetcd 定位为简化的 KV + Watch 封装，不提供以下高级功能：
//   - 事务（Txn/CAS/PutIfAbsent）：通过 RawClient() 使用原生 etcd 事务 API
//   - 租约续约（KeepAlive）：通过 RawClient() 使用原生租约 API
//
// 这些功能的使用场景（服务注册、分布式选主等）需要更复杂的生命周期管理，
// 由调用方根据具体需求直接操作 RawClient() 更为灵活。
//
// # 与 xdlock 集成
//
// xetcd 提供的 Config 类型与 xdlock.EtcdConfig 兼容，可以复用配置。
package xetcd
