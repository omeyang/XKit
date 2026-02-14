package xlru

import (
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// Config 定义缓存配置。
type Config struct {
	// Size 缓存最大条目数。
	// 必须大于 0。
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
// 回调函数在缓存锁内执行，应避免耗时操作。
func WithOnEvicted[K comparable, V any](fn func(key K, value V)) Option[K, V] {
	return func(o *options[K, V]) {
		o.onEvicted = fn
	}
}

// Cache 是带 TTL 的 LRU 缓存。
// 所有方法都是并发安全的。
type Cache[K comparable, V any] struct {
	lru       *expirable.LRU[K, V]
	closeOnce sync.Once
}

// New 创建新的 LRU 缓存。
// 如果 cfg.Size <= 0，返回 ErrInvalidSize。
// 如果 cfg.TTL < 0，返回 ErrInvalidTTL。
func New[K comparable, V any](cfg Config, opts ...Option[K, V]) (*Cache[K, V], error) {
	if cfg.Size <= 0 {
		return nil, ErrInvalidSize
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
// 如果键不存在或已过期，返回零值和 false。
func (c *Cache[K, V]) Get(key K) (V, bool) {
	return c.lru.Get(key)
}

// Set 设置缓存值。
// 返回 true 表示添加新键时发生了淘汰（缓存已满）。
// 更新已存在的键不触发淘汰，返回 false。
func (c *Cache[K, V]) Set(key K, value V) bool {
	return c.lru.Add(key, value)
}

// Delete 删除缓存条目。
// 返回 true 表示键存在并被删除。
func (c *Cache[K, V]) Delete(key K) bool {
	return c.lru.Remove(key)
}

// Clear 清空所有缓存条目。
func (c *Cache[K, V]) Clear() {
	c.lru.Purge()
}

// Len 返回当前缓存条目数。
func (c *Cache[K, V]) Len() int {
	return c.lru.Len()
}

// Contains 检查键是否存在（不更新访问时间）。
func (c *Cache[K, V]) Contains(key K) bool {
	return c.lru.Contains(key)
}

// Keys 返回所有键的切片。
// 按从最旧到最新的顺序排列。
func (c *Cache[K, V]) Keys() []K {
	return c.lru.Keys()
}

// Close 关闭缓存，释放资源。
// 该方法是幂等的：多次调用只会执行一次清理。
//
// Close 会清空所有缓存条目并停止 TTL 过期清理 goroutine。
// 调用 Close 后不应再使用缓存。
func (c *Cache[K, V]) Close() {
	c.closeOnce.Do(func() {
		c.lru.Purge()
		stopCleanupGoroutine(c.lru)
	})
}

// stopCleanupGoroutine 停止 expirable.LRU 内部的清理 goroutine。
//
// hashicorp/golang-lru/v2@v2.0.7 在 TTL > 0 时启动后台 goroutine 清理过期条目，
// 但其 Close() 方法被注释掉（计划在 v2.1 中实现），无法通过公开 API 停止。
// 此函数通过 reflect + unsafe 访问内部 done 通道并关闭它。
//
// 安全保证：
//   - 如果上游库结构变化（字段重命名/类型变更），静默降级为无操作
//   - 如果 done 已关闭，recover 捕获 panic，不会崩溃
func stopCleanupGoroutine(lru any) {
	defer func() { recover() }() //nolint:errcheck // recover 返回值无需处理

	v := reflect.ValueOf(lru)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}

	doneField := v.Elem().FieldByName("done")
	if !doneField.IsValid() || doneField.IsNil() {
		return
	}

	// 验证字段类型为 chan struct{}
	if doneField.Type() != reflect.TypeOf(make(chan struct{})) {
		return
	}

	// 通过 unsafe 访问未导出字段值，关闭 done 通道使清理 goroutine 退出
	doneCh := *(*chan struct{})(unsafe.Pointer(doneField.UnsafeAddr())) //nolint:gosec // 有意使用 unsafe 访问内部字段
	close(doneCh)
}
