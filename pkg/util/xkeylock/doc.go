// Package xkeylock 提供基于 key 的进程内互斥锁。
//
// 适用于需要按业务 key 进行互斥的场景，如资产创建互斥、风险更新互斥等。
//
// # 与 xdlock 的区别
//
//	特性          xkeylock              xdlock
//	──────────────────────────────────────────
//	范围          进程内                 分布式（Redis/etcd）
//	Context       ✓ Acquire 支持       ✓
//	TryAcquire    ✓                    ✓ TryLock
//	Handle        Unlock()+Key()       Unlock(ctx)+Extend(ctx)+Key()
//	性能          纳秒级（内存操作）     毫秒级（网络调用）
//
// # 特性
//
//   - Context 支持：Acquire 支持超时和取消（ctx 不得为 nil，否则 panic）
//   - Key 校验：空字符串 key 立即返回 ErrInvalidKey（fail-fast，防止误用）
//   - TryAcquire：非阻塞获取，锁被占用时返回 ErrLockOccupied
//   - Handle 语义：Unlock 幂等（首次返回 nil，后续返回 ErrLockNotHeld）
//   - 分片 map：默认 32 分片（上限 65536），减少管理锁争用
//   - 内存安全：WithMaxKeys(n) 可限制最大 key 数
//   - 关闭语义：Close() 拒绝新请求，已持有锁不受影响
//   - Close() 唤醒所有等待中的 Acquire，使其返回 ErrClosed
//   - 非可重入：同一 goroutine 对同一 key 重复 Acquire 会死锁，
//     建议始终使用带 deadline 的 context 以防意外阻塞
//   - 近似公平：等待者按 Go channel 内部队列顺序唤醒（近似 FIFO），
//     但不提供严格公平性保证；极端高并发下可能存在微弱的唤醒不均
package xkeylock
