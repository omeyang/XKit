// Package xdlock 提供分布式锁的统一封装，支持 etcd 和 Redis (Redlock) 后端。
//
// # 设计理念
//
// xdlock 采用与 xcache、xmq 相同的设计模式：
//   - 工厂函数：NewEtcdFactory, NewRedisFactory
//   - 底层暴露：Session()/Redsync() 直接返回底层实例，不限制任何底层特性
//   - 增值功能：健康检查、统一错误处理、输入校验
//
// 这种设计让用户可以：
//   - 使用熟悉的底层 API，无需学习新抽象
//   - 获得开箱即用的运维能力（健康检查、统一错误、输入校验）
//   - 在需要时直接访问底层库的高级特性
//
// # 核心概念
//
//   - Factory: 锁工厂，管理连接并提供 TryLock/Lock 操作
//   - LockHandle: 单次锁获取的句柄，提供 Unlock/Extend/Key 操作
//   - MutexOption: 锁实例的配置选项
//
// # etcd 后端
//
// 使用 NewEtcdClient 创建客户端，NewEtcdFactory 创建工厂。
// etcd 使用 Session KeepAlive 实现自动续期，无需像 Redis 那样手动调用 Extend 延长 TTL。
// 但长时间运行的任务应定期调用 Extend 检测 Session 健康状态（见 LockHandle.Extend 文档）。
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
//	| Extend() | 检查 Session 健康状态和本地解锁标记（不延长 TTL） | 延长锁 TTL |
//	| 多节点支持 | 原生（etcd 集群） | Redlock 算法 |
//	| 锁释放 | 立即生效 | 立即生效 |
//	| MutexOption | 仅 KeyPrefix 生效 | 全部生效 |
//
// # Factory 关闭行为
//
// Redis: Factory.Close(ctx) 仅阻止创建新锁，已持有的 LockHandle 仍可执行 Unlock/Extend。
// 这避免了关闭流程先于业务 Unlock 发生时锁悬挂等待 TTL 过期的问题。
//
// etcd: Factory.Close(ctx) 会关闭底层 Session 并撤销 Lease，所有基于该 Session 的锁
// 会立即失效。已持有的 LockHandle 在 Close 后 Extend 会返回 ErrSessionExpired，
// Unlock 可能返回错误但锁已自动释放，不会悬挂。
//
// # Unlock 清理上下文
//
// 设计决策: Unlock 使用独立清理上下文。当调用方的 context 已取消/超时时（如 defer
// handle.Unlock(ctx) 中 ctx 已过期），Unlock 会自动切换到 context.Background() 派生的
// 5 秒超时上下文，确保解锁操作尽力完成，避免锁残留到 TTL/Lease 到期。
//
// # Key 校验
//
// 锁 key 必须满足：非空（去除空白后不为空）、长度不超过 512 字节。
// 超长 key 会返回 [ErrKeyTooLong]。
//
// # 锁重入
//
// 设计决策: xdlock 的锁是非重入的。
//
// etcd 后端：同一 Factory（同一 Session）对同一 key 只能持有一个 LockHandle。
// etcd concurrency.Mutex 的所有权标识由 Session Lease 决定，同一 Session 对同一
// key prefix 创建的 Mutex 共享相同的 owner key（pfx + hex(leaseID)）。
// 为防止多个 handle 错误共享所有权（其中一个 Unlock 会释放所有 handle 的锁），
// Factory 使用本地追踪机制：TryLock 返回 (nil, nil)，Lock 返回 ErrLockFailed。
//
// Redis 后端：每个 LockHandle 通过随机值（redsync 默认）实现独立所有权，
// 同一 Factory 可对同一 key 创建多个独立 handle（前提是锁未被占用）。
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
