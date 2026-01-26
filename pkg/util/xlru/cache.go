package xlru

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// Config 定义缓存配置。
type Config struct {
	// Size 缓存最大条目数。
	// 必须大于 0。
	Size int

	// TTL 条目过期时间。
	// 0 表示永不过期。
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
	lru *expirable.LRU[K, V]
}

// New 创建新的 LRU 缓存。
// 如果 cfg.Size <= 0，返回 ErrInvalidSize。
func New[K comparable, V any](cfg Config, opts ...Option[K, V]) (*Cache[K, V], error) {
	if cfg.Size <= 0 {
		return nil, ErrInvalidSize
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
