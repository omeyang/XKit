package xkeylock

import "fmt"

const (
	defaultShardCount = 32
	maxShardCount     = 1 << 16 // 65536
)

// Option 定义 Locker 可选配置。
type Option func(*options)

type options struct {
	maxKeys    int
	shardCount int
	shardMask  uint64 // validate() 计算，供 getShard 使用
}

func defaultOptions() options {
	return options{
		shardCount: defaultShardCount,
	}
}

// WithMaxKeys 设置最大 key 数量。
// 达到上限时，新的 Acquire/TryAcquire 返回 [ErrMaxKeysExceeded]。
// n <= 0 表示不限制（默认）。
func WithMaxKeys(n int) Option {
	// 在闭包外归一化，避免闭包写捕获变量导致并发复用时的数据竞争。
	if n < 0 {
		n = 0
	}
	return func(o *options) {
		o.maxKeys = n
	}
}

// WithShardCount 设置分片数量。
// 更多分片减少争用，但增加内存占用和 cache 开销。
// n 必须为正整数且为 2 的幂，上限 65536，否则 New 返回错误。默认 32。
// 建议设置为 2×GOMAXPROCS 左右；过多分片在 CPU 核数较少时无额外收益。
func WithShardCount(n int) Option {
	return func(o *options) {
		o.shardCount = n
	}
}

func (o *options) validate() error {
	sc := o.shardCount
	if sc <= 0 || sc > maxShardCount || sc&(sc-1) != 0 {
		return fmt.Errorf("%w: must be a positive power of 2 (max %d), got %d",
			ErrInvalidShardCount, maxShardCount, sc)
	}
	// sc ∈ [1, maxShardCount] 且为 2 的幂，int→uint64 转换安全。
	o.shardMask = uint64(sc - 1)
	return nil
}
