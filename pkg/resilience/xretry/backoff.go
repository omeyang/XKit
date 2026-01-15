package xretry

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	mathrand "math/rand/v2"
	"time"
)

// FixedBackoff 固定延迟退避策略
type FixedBackoff struct {
	delay time.Duration
}

// NewFixedBackoff 创建固定延迟退避策略
func NewFixedBackoff(delay time.Duration) *FixedBackoff {
	if delay < 0 {
		delay = 0
	}
	return &FixedBackoff{delay: delay}
}

func (b *FixedBackoff) NextDelay(_ int) time.Duration {
	return b.delay
}

// ExponentialBackoff 指数退避策略
// delay = min(initialDelay * multiplier^(attempt-1) * (1 + jitter), maxDelay)
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
	jitter       float64
}

// ExponentialBackoffOption 指数退避配置选项
type ExponentialBackoffOption func(*ExponentialBackoff)

// WithInitialDelay 设置初始延迟
func WithInitialDelay(d time.Duration) ExponentialBackoffOption {
	return func(b *ExponentialBackoff) {
		if d > 0 {
			b.initialDelay = d
		}
	}
}

// WithMaxDelay 设置最大延迟
func WithMaxDelay(d time.Duration) ExponentialBackoffOption {
	return func(b *ExponentialBackoff) {
		if d > 0 {
			b.maxDelay = d
		}
	}
}

// WithMultiplier 设置乘数因子
func WithMultiplier(m float64) ExponentialBackoffOption {
	return func(b *ExponentialBackoff) {
		if m > 1 {
			b.multiplier = m
		}
	}
}

// WithJitter 设置抖动因子（0-1 之间）
func WithJitter(j float64) ExponentialBackoffOption {
	return func(b *ExponentialBackoff) {
		if j < 0 {
			j = 0
		} else if j > 1 {
			j = 1
		}
		b.jitter = j
	}
}

// NewExponentialBackoff 创建指数退避策略
// 默认值：
//   - initialDelay: 100ms
//   - maxDelay: 30s
//   - multiplier: 2.0
//   - jitter: 0.1 (10%)
func NewExponentialBackoff(opts ...ExponentialBackoffOption) *ExponentialBackoff {
	b := &ExponentialBackoff{
		initialDelay: 100 * time.Millisecond,
		maxDelay:     30 * time.Second,
		multiplier:   2.0,
		jitter:       0.1,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func (b *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	// 计算基础延迟
	delay := float64(b.initialDelay) * math.Pow(b.multiplier, float64(attempt-1))

	// 添加抖动
	if b.jitter > 0 {
		jitterFactor := 1.0 + (randomFloat64()*2-1)*b.jitter
		delay *= jitterFactor
	}

	// 限制最大延迟
	if delay > float64(b.maxDelay) {
		delay = float64(b.maxDelay)
	}

	return time.Duration(delay)
}

func (b *ExponentialBackoff) Reset() {
	// crypto/rand 不需要重置
}

// LinearBackoff 线性退避策略
// delay = min(initialDelay + increment * (attempt-1), maxDelay)
type LinearBackoff struct {
	initialDelay time.Duration
	increment    time.Duration
	maxDelay     time.Duration
}

// NewLinearBackoff 创建线性退避策略
func NewLinearBackoff(initialDelay, increment, maxDelay time.Duration) *LinearBackoff {
	if initialDelay < 0 {
		initialDelay = 0
	}
	if increment < 0 {
		increment = 0
	}
	if maxDelay < initialDelay {
		maxDelay = initialDelay
	}
	return &LinearBackoff{
		initialDelay: initialDelay,
		increment:    increment,
		maxDelay:     maxDelay,
	}
}

func (b *LinearBackoff) NextDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	// 预检查：如果 attempt 过大，直接返回 maxDelay
	// 避免 attempt-1 或后续乘法溢出成小正数绕过检测
	// 10 亿次重试已远超任何实际场景
	const maxSafeAttempt = 1 << 30
	if attempt > maxSafeAttempt {
		return b.maxDelay
	}

	// 计算增量，防止整数溢出
	incrementPart := b.increment * time.Duration(attempt-1)
	// 如果增量为负（溢出）或结果会溢出，直接返回 maxDelay
	if incrementPart < 0 || b.initialDelay > b.maxDelay-incrementPart {
		return b.maxDelay
	}

	delay := b.initialDelay + incrementPart
	if delay > b.maxDelay {
		delay = b.maxDelay
	}
	return delay
}

// NoBackoff 无延迟退避策略
type NoBackoff struct{}

// NewNoBackoff 创建无延迟退避策略
func NewNoBackoff() *NoBackoff {
	return &NoBackoff{}
}

func (b *NoBackoff) NextDelay(_ int) time.Duration {
	return 0
}

// 确保实现了 BackoffPolicy 接口
var (
	_ BackoffPolicy      = (*FixedBackoff)(nil)
	_ BackoffPolicy      = (*ExponentialBackoff)(nil)
	_ BackoffPolicy      = (*LinearBackoff)(nil)
	_ BackoffPolicy      = (*NoBackoff)(nil)
	_ ResettableBackoff  = (*ExponentialBackoff)(nil)
)

const (
	floatBits  = 53
	floatScale = 1.0 / (1 << floatBits)
)

func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return float64(binary.LittleEndian.Uint64(buf[:])>>11) * floatScale
	}
	// crypto/rand 失败时回退到 math/rand（极少发生）
	return mathrand.Float64() //nolint:gosec // G404: fallback 场景，性能优先
}
