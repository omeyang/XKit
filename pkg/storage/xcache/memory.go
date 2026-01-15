package xcache

import (
	"github.com/dgraph-io/ristretto/v2"
)

// =============================================================================
// Memory 接口定义
// =============================================================================

// Memory 定义内存缓存接口。
// 只提供 ristretto 原生不便使用的增值功能，基础操作请直接使用 Client()。
type Memory interface {
	// Stats 返回缓存统计信息。
	Stats() MemoryStats

	// Client 返回底层的 ristretto.Cache。
	// 用于执行所有缓存操作。
	Client() *ristretto.Cache[string, []byte]

	// Wait 等待所有缓冲的写入完成。
	// ristretto 使用异步写入，调用此方法确保写入完成。
	Wait()

	// Close 关闭缓存。
	Close()
}

// =============================================================================
// 统计信息
// =============================================================================

// MemoryStats 定义内存缓存的统计信息。
type MemoryStats struct {
	// Hits 缓存命中次数。
	Hits uint64

	// Misses 缓存未命中次数。
	Misses uint64

	// HitRatio 缓存命中率 (0.0 - 1.0)。
	HitRatio float64

	// KeysAdded 已添加的 key 数量。
	KeysAdded uint64

	// KeysEvicted 已淘汰的 key 数量。
	KeysEvicted uint64

	// CostAdded 已添加的总 cost。
	CostAdded uint64

	// CostEvicted 已淘汰的总 cost。
	CostEvicted uint64
}

// =============================================================================
// Memory 配置选项
// =============================================================================

// MemoryOptions 定义内存缓存的配置选项。
type MemoryOptions struct {
	// NumCounters 用于跟踪频率的计数器数量。
	// 建议设置为预期 key 数量的 10 倍。
	// 默认为 1e7 (10M)。
	NumCounters int64

	// MaxCost 缓存的最大容量（字节）。
	// 默认为 100MB。
	MaxCost int64

	// BufferItems 写入缓冲区的大小。
	// 默认为 64。
	BufferItems int64
}

// MemoryOption 定义配置内存缓存的函数类型。
type MemoryOption func(*MemoryOptions)

// defaultMemoryOptions 返回默认的内存缓存配置。
func defaultMemoryOptions() *MemoryOptions {
	return &MemoryOptions{
		NumCounters: 1e7,               // 10M counters
		MaxCost:     100 * 1024 * 1024, // 100MB
		BufferItems: 64,
	}
}

// WithMemoryNumCounters 设置计数器数量。
func WithMemoryNumCounters(n int64) MemoryOption {
	return func(o *MemoryOptions) {
		o.NumCounters = n
	}
}

// WithMemoryMaxCost 设置最大容量（字节）。
func WithMemoryMaxCost(cost int64) MemoryOption {
	return func(o *MemoryOptions) {
		o.MaxCost = cost
	}
}

// WithMemoryBufferItems 设置写入缓冲区大小。
func WithMemoryBufferItems(n int64) MemoryOption {
	return func(o *MemoryOptions) {
		o.BufferItems = n
	}
}
