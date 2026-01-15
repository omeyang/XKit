// Package xdlock 提供分布式锁的统一封装，支持 etcd 和 Redis (Redlock) 后端。
//
// # 设计理念
//
// xdlock 采用与 xcache、xmq 相同的设计模式：
//   - 工厂函数：NewEtcdFactory, NewRedisFactory
//   - 底层暴露：Session()/Redsync()/Mutex() 直接返回底层实例，不限制任何底层特性
//   - 增值功能：健康检查、统一错误处理、可观测性
//
// 这种设计让用户可以：
//   - 使用熟悉的底层 API，无需学习新抽象
//   - 获得开箱即用的运维能力（健康检查、统一错误）
//   - 在需要时直接访问底层库的高级特性
//
// # 核心概念
//
//   - Factory: 锁工厂，管理连接和创建锁实例
//   - Locker: 单个分布式锁实例，提供 Lock/TryLock/Unlock/Extend 操作
//   - MutexOption: 锁实例的配置选项
//
// # etcd 后端
//
// 使用 NewEtcdClient 创建客户端，NewEtcdFactory 创建工厂。
// etcd 使用 Session 实现自动续期，无需手动调用 Extend。
// 也可使用 NewEtcdFactoryFromConfig 一步创建客户端和工厂。
//
// # Redis 后端
//
// 使用 NewRedisFactory 创建工厂，支持单节点和 Redlock 多节点模式。
// Redis 需要手动调用 Extend 进行续期。生产环境推荐使用 Redlock 多节点模式。
//
// # 后端差异
//
//	| 特性 | etcd | Redis (redsync) |
//	|------|------|-----------------|
//	| 续期方式 | 自动（Session） | 手动（Extend） |
//	| Extend() | 检查 Session 状态（返回 nil 或 ErrSessionExpired） | 延长锁 TTL |
//	| 多节点支持 | 原生（etcd 集群） | Redlock 算法 |
//	| 锁释放 | 立即生效 | 立即生效 |
//
// # 与 xcache 的关系
//
// xcache 和 xdlock 各自独立实现，针对不同场景：
//
//   - xcache 内置锁：简单实现（SET NX EX），适用于缓存防击穿的辅助场景
//   - xdlock：完整分布式锁，适用于需要互斥保护的业务场景
//
// 对于需要更强一致性的缓存场景，可通过 xcache.WithExternalLock 集成 xdlock。
//
// 选择建议：
//
//	| 场景 | 推荐方案 |
//	|------|----------|
//	| 简单缓存防击穿 | xcache 内置锁（默认） |
//	| 高可用缓存防击穿 | xcache + xdlock（通过 WithExternalLock） |
//	| 业务互斥（单节点） | xdlock.NewRedisFactory(client) |
//	| 业务互斥（多节点） | xdlock.NewRedisFactory(client1, client2, client3) |
//	| 强一致性互斥 | xdlock.NewEtcdFactory(etcdClient) |
//
// 详细使用示例请参考 example_test.go 中的 Example 函数。
package xdlock
