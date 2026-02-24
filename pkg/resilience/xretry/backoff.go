package xretry

import (
	"crypto/rand"
	"encoding/binary"
	"math"
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
// delay = min(initialDelay * multiplier^(attempt-1) * (1 + rand(-1,1) * jitter), maxDelay)
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
	jitter       float64
}

// ExponentialBackoffOption 指数退避配置选项
type ExponentialBackoffOption func(*ExponentialBackoff)

// WithInitialDelay 设置初始延迟。
// 设计决策: d <= 0 时静默忽略（保持默认值），与 WithMaxDelay/WithMultiplier 一致。
// WithJitter 则采用 clamp 策略，因为 jitter 有明确的有效区间 [0,1]。
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

// WithMultiplier 设置乘数因子（>= 1.0）
// 传入 1.0 表示固定延迟（无指数增长）。
// 小于 1.0 的值会被忽略（保持默认值 2.0）。
func WithMultiplier(m float64) ExponentialBackoffOption {
	return func(b *ExponentialBackoff) {
		if m >= 1 {
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
	// 与 NewLinearBackoff 保持一致：确保 maxDelay >= initialDelay
	if b.maxDelay < b.initialDelay {
		b.maxDelay = b.initialDelay
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

	// 设计决策: NaN 安全的延迟限制。当 attempt 极大时 math.Pow 溢出为 +Inf，
	// 与 jitterFactor=0 相乘产生 NaN。IEEE 754 中 NaN 的所有比较均返回 false，
	// 会绕过 maxDelay 限制。NaN/负数返回 maxDelay（语义为退避已达上限）。
	if math.IsNaN(delay) || delay < 0 {
		return b.maxDelay
	}
	if delay >= float64(b.maxDelay) {
		return b.maxDelay
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

	// 安全溢出检测：通过预计算最大允许的乘数来避免溢出，在溢出发生前就进行检测
	//
	// 原理：如果 increment * (attempt-1) > maxDelay - initialDelay，
	// 则结果必定超过 maxDelay，应直接返回 maxDelay。
	// 通过计算 maxMultiplier = (maxDelay - initialDelay) / increment，
	// 可以在不发生溢出的情况下判断是否会超限。
	if b.increment > 0 && attempt > 1 {
		available := b.maxDelay - b.initialDelay
		if available < 0 {
			// 设计决策: 防御性检查——构造函数已确保 maxDelay >= initialDelay，
			// 但此守卫保护直接构造（绕过工厂）的场景。
			return b.maxDelay
		}
		maxMultiplier := available / b.increment
		if time.Duration(attempt-1) > maxMultiplier {
			return b.maxDelay
		}
	}

	// 此时可以安全计算，不会溢出
	incrementPart := b.increment * time.Duration(attempt-1)
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

// 确保实现了接口
var (
	_ BackoffPolicy = (*FixedBackoff)(nil)
	_ BackoffPolicy = (*ExponentialBackoff)(nil)
	_ BackoffPolicy = (*LinearBackoff)(nil)
	_ BackoffPolicy = (*NoBackoff)(nil)
)

const (
	floatBits  = 53
	floatScale = 1.0 / (1 << floatBits)
)

func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败时返回 0，这意味着无抖动（安全默认值）
		return 0
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) * floatScale
}
