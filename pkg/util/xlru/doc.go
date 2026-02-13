// Package xlru 提供带 TTL 的 LRU 缓存实现。
//
// xlru 基于 github.com/hashicorp/golang-lru/v2/expirable 封装，
// 提供简洁的泛型 API，适合作为本地缓存层使用。
//
// # 核心特性
//
//   - 泛型支持：支持任意 comparable 的键类型和任意值类型
//   - TTL 过期：条目超过 TTL 自动过期，Get 时返回 miss
//   - LRU 淘汰：缓存满时自动淘汰最久未访问的条目
//   - 并发安全：所有操作都是线程安全的
//
// # 配置选项
//
// Config 结构体提供必需的配置：
//   - Size：缓存最大条目数，必须 > 0 且 ≤ 16,777,216
//   - TTL：条目过期时间，0 表示永不过期
//
// 可选配置通过 Option 函数提供：
//   - WithOnEvicted：设置条目被淘汰时的回调函数
//
// # 使用场景
//
// xlru 适合以下场景：
//   - 本地缓存层（L1），配合 Redis 等分布式缓存（L2）使用
//   - 高频读取的小数据集缓存
//   - Token、Session 等有时效性的数据缓存
//   - 配置数据的本地副本
//
// # 性能特性
//
//   - Get/Set 操作 O(1) 时间复杂度
//   - Keys() 会分配新切片，复杂度 O(n)
//   - 并发安全由底层库保证，使用 sync.Mutex
//
// # 设计决策
//
// xlru 是对 hashicorp/golang-lru/v2/expirable 的轻量封装，
// 不提供接口抽象层。这是有意的设计选择：
//   - 保持 API 简洁，避免过度抽象
//   - 底层库稳定成熟，替换需求极低
//   - 如需替换实现，建议在业务层封装
//
// # 已知限制
//
//   - 不支持自定义时钟：TTL 使用系统时间，无法注入 mock 时钟
//   - 无内置指标：不提供命中率、淘汰次数等统计
//   - Clear() 会触发 OnEvicted：调用 Clear() 时，所有条目的淘汰回调都会被调用
//   - TTL 延迟清理语义：Get/Peek 会过滤已过期条目，但 Contains/Len/Keys 可能包含
//     已过期但尚未被后台清理的条目（底层库行为）
//   - 锁竞争：底层库使用 sync.Mutex（非 RWMutex），因为 Get 会更新 LRU 顺序；
//     高并发读场景下可能有锁竞争，当前性能对大多数场景足够
//   - Close 后行为：Close 后所有读操作返回零值/false，写操作静默忽略
//   - unsafe 依赖：Close 通过 reflect+unsafe 访问底层库未导出字段以停止 goroutine，
//     升级 golang-lru 版本时需验证兼容性
//
// # 注意事项
//
//   - TTL 是条目级别的，从 Set 时刻开始计算
//   - Set 覆盖已有 key 时会刷新 TTL
//   - Get 不会刷新 TTL（与某些缓存库的行为不同）
//   - Size 是条目数量，不是内存大小
//   - 淘汰回调在锁内执行，严禁在回调中调用 Cache 自身方法（会死锁），应避免耗时操作
//   - 使用完毕后应调用 Close() 释放清理 goroutine，避免泄漏
package xlru
