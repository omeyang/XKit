// Package xcache 提供统一的缓存工厂和增值功能，支持 Redis 和内存缓存。
//
// # 设计理念
//
// xcache 不包装底层客户端的所有 API，而是提供：
//   - 统一的工厂方法（NewRedis, NewMemory）
//   - 底层客户端直接暴露（Client() 方法）
//   - 增值功能（Loader 模式、分布式锁）
//
// # 核心组件
//
//   - Redis：暴露 go-redis UniversalClient，提供分布式锁
//   - Memory：暴露 ristretto Cache，提供统计信息
//   - Loader：Cache-Aside 模式加载器，内置 singleflight + 分布式锁防击穿
//
// # 快速开始
//
// 使用 NewRedis 创建 Redis 缓存，通过 Client() 获取底层 go-redis 客户端。
// 使用 NewMemory 创建内存缓存，通过 Client() 获取底层 ristretto 客户端。
// 使用 NewLoader 创建 Cache-Aside 加载器。
//
// 详细使用示例参考 example_test.go。
//
// # Context 安全
//
// 所有接受 context.Context 的公开入口方法（Load, LoadHash, Lock）
// 均在入口处检查 nil context，传入 nil 会返回 ErrNilContext 而非 panic。
//
// # Loader Context 处理
//
// Loader 使用 singleflight 合并并发请求时，内部使用独立 context：
//   - 第一个 caller cancel 不影响其他 caller 获取结果
//   - 默认超时 30 秒（可通过 WithLoadTimeout 配置）
//
// # Panic 安全
//
// loadFn（用户提供的回源函数）如果发生 panic，xcache 会将其捕获并转为
// ErrLoadPanic 错误返回，避免在 singleflight DoChan 模式下导致进程崩溃。
//
// # Singleflight 去重说明
//
// singleflight 去重仅基于 key（对于 LoadHash 是 key+field），不包含 ttl。
// 这意味着同一 key 的并发请求（即使 ttl 不同）只会触发一次回源，
// 最终缓存的 TTL 取决于首个请求的配置。
// 这是设计决策：同一数据应使用一致的 TTL 配置。
//
// # loadFn 返回值说明
//
// loadFn 返回 (nil, nil) 时，nil 值会被写入 Redis（存储为空字符串），
// 后续 Get 返回 []byte("")，视为缓存命中。
// 这等效于空值缓存，可防止缓存穿透。
//
// # TTL 抖动
//
// 使用 WithTTLJitter 可为缓存 TTL 添加随机抖动，防止大量 key 同时过期（缓存雪崩）。
// 例如 WithTTLJitter(0.1) 时，1 小时 TTL 会被随机化到约 57-63 分钟。
//
// # 分布式锁
//
// 锁 key 格式：lock:{prefix}{key}
//   - {prefix} 默认为 "loader:"，可通过 WithDistributedLockKeyPrefix 配置
//
// 分布式锁的目的是减轻后端压力（防击穿），而非保证缓存强一致性。
// 如果锁在加载完成前过期，其他节点可能并发回源，但这不会导致数据错误，
// 只是降低了防击穿效果。合理配置 DistributedLockTTL > LoadTimeout 可极大降低此概率。
//
// 对于需要更强一致性的场景，可通过 WithExternalLock 集成 xdlock。
//
// # Memory 组件说明
//
// Memory 是独立的本地内存缓存包装（基于 ristretto），不参与 Loader 流程。
// Loader 仅支持 Redis 后端。如需本地缓存 + Redis 的双层缓存，
// 应在业务层自行组合 Memory 和 Loader。
package xcache
