package xkeylock

const (
	defaultShardCount = 32
)

// Option 定义 KeyLock 可选配置。
type Option func(*options)

type options struct {
	maxKeys    int
	shardCount uint
}

func defaultOptions() *options {
	return &options{
		shardCount: defaultShardCount,
	}
}

// WithMaxKeys 设置最大 key 数量。
// 达到上限时，新的 Acquire/TryAcquire 返回 [ErrMaxKeysExceeded]。
// n <= 0 表示不限制（默认）。
func WithMaxKeys(n int) Option {
	return func(o *options) {
		if n < 0 {
			n = 0
		}
		o.maxKeys = n
	}
}

// WithShardCount 设置分片数量。
// 更多分片减少争用，但增加内存占用。
// n 必须为正整数且为 2 的幂，否则 panic。默认 32。
// 使用 uint 类型：负数在编译期即报错，比运行时 panic 更安全。
func WithShardCount(n uint) Option {
	if n == 0 || n&(n-1) != 0 {
		panic("xkeylock: shard count must be a positive power of 2")
	}
	return func(o *options) {
		o.shardCount = n
	}
}
