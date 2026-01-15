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
// # Loader Context 处理
//
// Loader 使用 singleflight 合并并发请求时，内部使用独立 context：
//   - 第一个 caller cancel 不影响其他 caller 获取结果
//   - 默认超时 30 秒（可通过 WithLoadTimeout 配置）
//
// # 分布式锁
//
// 锁 key 格式：lock:{prefix}{key}
//   - {prefix} 默认为 "loader:"，可通过 WithDistributedLockKeyPrefix 配置
//
// 对于需要更强一致性的场景，可通过 WithExternalLock 集成 xdlock。
package xcache
