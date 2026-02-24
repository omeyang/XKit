package xlru

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// maxSize 缓存最大条目数上限。
const maxSize = 1 << 24 // 16,777,216

// Config 定义缓存配置。
type Config struct {
	// Size 缓存最大条目数。
	// 必须大于 0 且不超过 16,777,216。
	Size int

	// TTL 条目过期时间。
	// 0 表示永不过期，不允许负值。
	TTL time.Duration
}

// Option 定义缓存可选配置函数类型。
type Option[K comparable, V any] func(*options[K, V])

// options 内部可选配置。
type options[K comparable, V any] struct {
	onEvicted func(key K, value V)
}

// WithOnEvicted 设置条目被淘汰时的回调函数。
//
// 设计决策: 回调在底层库的互斥锁内同步执行（Set 触发淘汰、Clear、Close 等路径均会调用）。
// 调用方必须遵守以下约束：
//   - 严禁在回调中调用 Cache 自身的任何方法（Get/Set/Delete/Len 等），否则会死锁
//   - 应避免耗时操作（如网络 I/O），以免阻塞其他并发操作
//   - 如需在回调中执行复杂逻辑，应将事件发送到外部 channel 异步处理
func WithOnEvicted[K comparable, V any](fn func(key K, value V)) Option[K, V] {
	return func(o *options[K, V]) {
		o.onEvicted = fn
	}
}

// Cache 是带 TTL 的 LRU 缓存。
// 必须通过 [New] 函数创建，零值不可用（方法调用会 panic）。
// 所有方法都是并发安全的。
// 调用 Close 后，所有读操作返回零值/false，写操作静默忽略。
type Cache[K comparable, V any] struct {
	lru       *expirable.LRU[K, V]
	closed    atomic.Bool
	closeOnce sync.Once
}

// New 创建新的 LRU 缓存。
// 如果 cfg.Size <= 0，返回 ErrInvalidSize。
// 如果 cfg.Size > maxSize (16,777,216)，返回 ErrSizeExceedsMax。
// 如果 cfg.TTL < 0，返回 ErrInvalidTTL。
func New[K comparable, V any](cfg Config, opts ...Option[K, V]) (*Cache[K, V], error) {
	if cfg.Size <= 0 {
		return nil, ErrInvalidSize
	}
	if cfg.Size > maxSize {
		return nil, ErrSizeExceedsMax
	}
	if cfg.TTL < 0 {
		return nil, ErrInvalidTTL
	}

	// 应用可选配置
	o := &options[K, V]{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	// 创建 expirable LRU
	lru := expirable.NewLRU(cfg.Size, o.onEvicted, cfg.TTL)

	return &Cache[K, V]{
		lru: lru,
	}, nil
}

// Get 获取缓存值。
// 如果键不存在、已过期或缓存已关闭，返回零值和 false。
func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	if c.closed.Load() {
		return value, false
	}
	return c.lru.Get(key)
}

// Set 设置缓存值。返回值表示是否触发了 LRU 淘汰（eviction），而非操作是否成功。
//
// 设计决策: 返回 bool 透传底层 expirable.LRU.Add 的淘汰语义，便于调用方
// 在需要时感知淘汰事件（如日志记录）。大多数场景可忽略返回值。
//
//   - 如果 key 已存在，更新值并刷新 TTL，不触发淘汰，返回 false
//   - 如果 key 不存在且缓存已满，淘汰最久未访问的条目，返回 true
//   - 如果缓存已关闭，静默忽略并返回 false
func (c *Cache[K, V]) Set(key K, value V) bool {
	if c.closed.Load() {
		return false
	}
	return c.lru.Add(key, value)
}

// Delete 删除缓存条目。
// 返回 true 表示键存在并被删除。
// 如果缓存已关闭，返回 false。
func (c *Cache[K, V]) Delete(key K) bool {
	if c.closed.Load() {
		return false
	}
	return c.lru.Remove(key)
}

// Clear 清空所有缓存条目。
// 如果缓存已关闭，静默忽略。
func (c *Cache[K, V]) Clear() {
	if c.closed.Load() {
		return
	}
	c.lru.Purge()
}

// Len 返回当前缓存条目数。
//
// 注意：返回值可能包含已过期但尚未被后台清理的条目。
// 如果缓存已关闭，返回 0。
func (c *Cache[K, V]) Len() int {
	if c.closed.Load() {
		return 0
	}
	return c.lru.Len()
}

// Contains 检查键是否存在（不更新访问时间）。
//
// 设计决策: 内部使用 Peek 而非上游 expirable.LRU.Contains，因为上游 Contains
// 不检查 TTL 过期（仅做 map 查找），而 Peek 会过滤已过期条目。
// 这确保 Contains 与 Get/Peek 的 TTL 语义一致。
//
// 如果缓存已关闭，返回 false。
func (c *Cache[K, V]) Contains(key K) bool {
	if c.closed.Load() {
		return false
	}
	_, ok := c.lru.Peek(key)
	return ok
}

// Peek 获取缓存值但不更新 LRU 顺序。
// 适用于检查缓存状态而不影响淘汰策略的场景。
// 如果缓存已关闭，返回零值和 false。
func (c *Cache[K, V]) Peek(key K) (value V, ok bool) {
	if c.closed.Load() {
		return value, false
	}
	return c.lru.Peek(key)
}

// Keys 返回所有键的切片，按从最旧到最新的顺序排列。
//
// 注意：返回值可能包含已过期但尚未被后台清理的条目的键。
// 如果缓存已关闭，返回 nil。
func (c *Cache[K, V]) Keys() []K {
	if c.closed.Load() {
		return nil
	}
	return c.lru.Keys()
}

// Close 关闭缓存，释放资源。
// 该方法是幂等的：多次调用只会执行一次清理。
//
// Close 会清空所有缓存条目并停止 TTL 过期清理 goroutine。
// Close 后所有读操作返回零值/false，写操作静默忽略。
//
// 设计决策: closed 标记与 lru 操作之间存在微小的 TOCTOU 窗口——在 closed.Load()
// 返回 false 到实际执行 lru 方法之间，另一个 goroutine 可能调用 Close()。
// 这是可接受的：底层 LRU 在 Purge() 后仍是有效对象（只是为空），
// 不会导致 panic 或数据损坏，仅可能出现操作在关闭瞬间的短暂可见性。
func (c *Cache[K, V]) Close() {
	c.closed.Store(true)
	c.closeOnce.Do(func() {
		c.lru.Purge()
		stopCleanupGoroutine(c.lru)
	})
}

// stopCleanupGoroutine 停止 expirable.LRU 内部的清理 goroutine。
// 返回 true 表示成功停止，false 表示降级为无操作（上游结构变化或通道已关闭）。
//
// 设计决策: hashicorp/golang-lru/v2@v2.0.7 在 TTL > 0 时启动后台 goroutine 清理过期条目，
// 但其 Close() 方法被注释掉（源码注释："decided to add functionality to close it in the version
// later than v2"），无法通过公开 API 停止。此函数通过 reflect + unsafe 访问内部 done 通道
// (类型 chan struct{}) 并关闭它，使后台 goroutine 退出。
//
// 已知限制：
//   - 依赖上游未导出字段 "done" 的名称和类型（chan struct{}），升级版本后应验证
//   - 如果上游结构变化（字段重命名/类型变更），返回 false（goroutine 泄漏），
//     此时 TestStopCleanupGoroutine_UpstreamStructAssert 会捕获此问题
//   - 如果 done 已关闭，recover 捕获 panic，返回 false
//
// 维护须知: 升级 golang-lru 版本时，检查上游是否已实现公开的 Close() 方法。
// 若已实现，应移除此函数并直接调用上游 Close()。
func stopCleanupGoroutine(lru any) (stopped bool) {
	defer func() {
		// close(doneCh) 可能因通道已关闭而 panic，静默捕获并返回 false
		if r := recover(); r != nil {
			stopped = false
		}
	}()

	v := reflect.ValueOf(lru)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}

	doneField := v.Elem().FieldByName("done")
	if !doneField.IsValid() || doneField.IsNil() {
		return false
	}

	// 验证字段类型为 chan struct{}
	if doneField.Type() != reflect.TypeOf(make(chan struct{})) {
		return false
	}

	// 通过 unsafe 访问未导出字段值，关闭 done 通道使清理 goroutine 退出
	doneCh := *(*chan struct{})(unsafe.Pointer(doneField.UnsafeAddr())) //nolint:gosec // 有意使用 unsafe 访问内部字段
	close(doneCh)
	return true
}
