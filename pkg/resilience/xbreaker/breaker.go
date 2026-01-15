package xbreaker

import (
	"context"
	"time"

	"github.com/sony/gobreaker/v2"
)

// TripPolicy 熔断判定策略接口
//
// 实现此接口可自定义熔断触发条件。
// 当 ReadyToTrip 返回 true 时，熔断器将从 Closed 状态转换为 Open 状态。
type TripPolicy interface {
	// ReadyToTrip 判断是否应该触发熔断
	// counts 包含当前统计窗口内的请求统计信息
	ReadyToTrip(counts Counts) bool
}

// SuccessPolicy 成功判定策略接口（可选）
//
// 实现此接口可自定义什么情况算作"成功"。
// 默认情况下，err == nil 即为成功。
type SuccessPolicy interface {
	// IsSuccessful 判断操作是否成功
	IsSuccessful(err error) bool
}

// Breaker 熔断器执行器
//
// Breaker 封装了 gobreaker 的熔断逻辑，提供更友好的 API。
// 使用 TripPolicy 接口抽象熔断判定，便于策略替换和测试。
type Breaker struct {
	name          string
	tripPolicy    TripPolicy
	successPolicy SuccessPolicy
	timeout       time.Duration
	interval      time.Duration
	bucketPeriod  time.Duration // 滑动窗口桶周期
	maxRequests   uint32
	onStateChange func(name string, from, to State)

	// 底层熔断器（延迟初始化）
	cb *gobreaker.CircuitBreaker[any]
}

// BreakerOption 熔断器配置选项
type BreakerOption func(*Breaker)

// WithTripPolicy 设置熔断判定策略
//
// 默认策略：连续失败 5 次触发熔断
func WithTripPolicy(p TripPolicy) BreakerOption {
	return func(b *Breaker) {
		if p != nil {
			b.tripPolicy = p
		}
	}
}

// WithSuccessPolicy 设置成功判定策略
//
// 默认情况下，err == nil 即为成功。
// 某些场景下可能需要自定义成功判定，例如 HTTP 5xx 算失败但 4xx 算成功。
func WithSuccessPolicy(p SuccessPolicy) BreakerOption {
	return func(b *Breaker) {
		b.successPolicy = p
	}
}

// WithTimeout 设置熔断器从 Open 状态恢复到 HalfOpen 状态的超时时间
//
// 默认值：60 秒
func WithTimeout(d time.Duration) BreakerOption {
	return func(b *Breaker) {
		if d > 0 {
			b.timeout = d
		}
	}
}

// WithInterval 设置统计窗口的周期（固定窗口模式）
//
// 在 Closed 状态下，每隔此周期会清除统计计数，重新开始统计。
// 这是固定窗口策略，窗口边界时刻可能产生统计偏差。
//
// 如果需要更平滑的滑动窗口策略，请同时配置 WithBucketPeriod。
//
// 默认值：0（不清除，持续累积）
//
// 示例：
//
//	// 固定窗口：每 60 秒清除一次统计
//	breaker := NewBreaker("my-service", WithInterval(60*time.Second))
//
//	// 滑动窗口：60 秒窗口，每 10 秒一个桶
//	breaker := NewBreaker("my-service",
//	    WithInterval(60*time.Second),
//	    WithBucketPeriod(10*time.Second),
//	)
func WithInterval(d time.Duration) BreakerOption {
	return func(b *Breaker) {
		b.interval = d
	}
}

// WithBucketPeriod 设置滑动窗口的桶周期
//
// 当同时配置 Interval 和 BucketPeriod 时，熔断器使用滑动窗口模式：
//   - Interval 定义整个窗口的时间跨度
//   - BucketPeriod 定义每个桶的时间跨度
//   - 窗口由 Interval/BucketPeriod 个桶组成
//
// 滑动窗口比固定窗口更平滑，避免窗口边界时刻的统计偏差。
//
// 注意：
//   - 如果未配置 WithInterval，此选项的行为依赖 gobreaker 内部实现
//   - 建议始终同时配置 WithInterval 以确保预期行为
//   - BucketPeriod 应能整除 Interval，否则会有行为偏差
//
// 默认值：0（使用固定窗口模式）
//
// 示例：
//
//	// 滑动窗口：60 秒窗口，每 10 秒一个桶（6个桶）
//	breaker := NewBreaker("my-service",
//	    WithInterval(60*time.Second),
//	    WithBucketPeriod(10*time.Second),
//	)
func WithBucketPeriod(d time.Duration) BreakerOption {
	return func(b *Breaker) {
		if d > 0 {
			b.bucketPeriod = d
		}
	}
}

// WithMaxRequests 设置 HalfOpen 状态下允许通过的最大请求数
//
// 默认值：1
func WithMaxRequests(n uint32) BreakerOption {
	return func(b *Breaker) {
		if n > 0 {
			b.maxRequests = n
		}
	}
}

// WithOnStateChange 设置状态变化回调
//
// 当熔断器状态发生变化时会调用此回调。
// 可用于日志记录、监控告警等。
func WithOnStateChange(f func(name string, from, to State)) BreakerOption {
	return func(b *Breaker) {
		b.onStateChange = f
	}
}

// NewBreaker 创建熔断器执行器
//
// name 是熔断器的名称，用于日志和监控标识。
// 默认配置：
//   - 熔断策略：连续失败 5 次触发熔断
//   - 超时时间：60 秒
//   - HalfOpen 最大请求数：1
func NewBreaker(name string, opts ...BreakerOption) *Breaker {
	b := &Breaker{
		name:        name,
		tripPolicy:  NewConsecutiveFailures(5), // 默认策略
		timeout:     60 * time.Second,
		maxRequests: 1,
	}

	for _, opt := range opts {
		opt(b)
	}

	// 初始化底层熔断器
	b.cb = b.buildCircuitBreaker()

	return b
}

// buildSettings 构建 gobreaker 配置
func (b *Breaker) buildSettings() gobreaker.Settings {
	st := gobreaker.Settings{
		Name:         b.name,
		MaxRequests:  b.maxRequests,
		Interval:     b.interval,
		BucketPeriod: b.bucketPeriod,
		Timeout:      b.timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return b.tripPolicy.ReadyToTrip(counts)
		},
	}

	// 如果有自定义成功判定策略
	if b.successPolicy != nil {
		st.IsSuccessful = func(err error) bool {
			return b.successPolicy.IsSuccessful(err)
		}
	}

	// 如果有状态变化回调
	if b.onStateChange != nil {
		st.OnStateChange = func(name string, from, to gobreaker.State) {
			b.onStateChange(name, from, to)
		}
	}

	return st
}

// buildCircuitBreaker 构建底层熔断器
func (b *Breaker) buildCircuitBreaker() *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](b.buildSettings())
}

// Do 执行受熔断器保护的操作
//
// 如果 context 已取消或超时，直接返回 context 错误。
// 如果熔断器处于 Open 状态，操作不会被执行，直接返回 ErrOpenState。
// 如果熔断器处于 HalfOpen 状态且请求过多，返回 ErrTooManyRequests。
//
// 注意：
//   - context 仅用于入口检查，不会传递给底层操作
//   - 熔断器错误会被包装为 BreakerError，实现 Retryable() 返回 false
//   - 这样在与 xretry 组合使用时，熔断错误不会被重试
func (b *Breaker) Do(ctx context.Context, fn func() error) error {
	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := b.cb.Execute(func() (any, error) {
		return nil, fn()
	})
	// 包装熔断器错误，使其实现 Retryable() 返回 false
	return wrapBreakerError(err, b.name, b.State())
}

// Execute 执行受熔断器保护的操作（泛型版本）
//
// 与 Do 类似，但支持返回值。
// 如果 context 已取消或超时，直接返回 context 错误。
//
// 注意：
//   - 此函数是包级函数而非方法，因为 Go 不支持方法的类型参数
//   - 熔断器错误会被包装为 BreakerError，实现 Retryable() 返回 false
func Execute[T any](ctx context.Context, b *Breaker, fn func() (T, error)) (T, error) {
	var zero T

	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	result, err := b.cb.Execute(func() (any, error) {
		return fn()
	})
	if err != nil {
		// 包装熔断器错误，使其实现 Retryable() 返回 false
		return zero, wrapBreakerError(err, b.name, b.State())
	}
	if result == nil {
		return zero, nil
	}
	if typed, ok := result.(T); ok {
		return typed, nil
	}
	// 类型断言失败，返回零值（理论上不会走到这里）
	return zero, nil
}

// State 返回熔断器当前状态
func (b *Breaker) State() State {
	return b.cb.State()
}

// Name 返回熔断器名称
func (b *Breaker) Name() string {
	return b.name
}

// Counts 返回当前统计计数
func (b *Breaker) Counts() Counts {
	return b.cb.Counts()
}

// CircuitBreaker 返回底层的 gobreaker.CircuitBreaker
//
// 通过此方法可以获取 gobreaker 的原生熔断器实例，
// 使用 gobreaker 的完整功能。
func (b *Breaker) CircuitBreaker() *gobreaker.CircuitBreaker[any] {
	return b.cb
}

// TripPolicy 返回当前熔断策略
func (b *Breaker) TripPolicy() TripPolicy {
	return b.tripPolicy
}

// SuccessPolicy 返回当前成功判定策略
//
// 如果未设置自定义策略，返回 nil（表示使用默认的 err == nil 判断）
func (b *Breaker) SuccessPolicy() SuccessPolicy {
	return b.successPolicy
}

// IsSuccessful 判断操作结果是否成功
//
// 如果设置了自定义 SuccessPolicy，使用它判断；否则使用默认的 err == nil 判断。
func (b *Breaker) IsSuccessful(err error) bool {
	if b.successPolicy != nil {
		return b.successPolicy.IsSuccessful(err)
	}
	return err == nil
}

// ManagedBreaker 托管的泛型熔断器
//
// 对于高性能场景，可以使用 ManagedBreaker 避免类型断言开销。
// ManagedBreaker 预设返回值类型，直接使用泛型熔断器。
type ManagedBreaker[T any] struct {
	breaker *Breaker
	cb      *gobreaker.CircuitBreaker[T]
}

// NewManagedBreaker 创建托管的泛型熔断器
//
// 使用已有的 Breaker 配置创建泛型熔断器。
// 适用于需要高性能且返回值类型固定的场景。
func NewManagedBreaker[T any](b *Breaker) *ManagedBreaker[T] {
	// 复用 Breaker 的配置
	st := b.buildSettings()

	return &ManagedBreaker[T]{
		breaker: b,
		cb:      gobreaker.NewCircuitBreaker[T](st),
	}
}

// Execute 执行受熔断器保护的操作
//
// 如果熔断器处于 Open 状态，操作不会被执行，直接返回 ErrOpenState。
// 如果熔断器处于 HalfOpen 状态且请求过多，返回 ErrTooManyRequests。
//
// 注意：
//   - 熔断器错误会被包装为 BreakerError，实现 Retryable() 返回 false
//   - 这样在与 xretry 组合使用时，熔断错误不会被重试
func (m *ManagedBreaker[T]) Execute(fn func() (T, error)) (T, error) {
	result, err := m.cb.Execute(fn)
	if err != nil {
		// 包装熔断器错误，使其实现 Retryable() 返回 false
		return result, wrapBreakerError(err, m.breaker.name, m.State())
	}
	return result, nil
}

// Name 返回熔断器名称
func (m *ManagedBreaker[T]) Name() string {
	return m.breaker.name
}

// State 返回熔断器当前状态
func (m *ManagedBreaker[T]) State() State {
	return m.cb.State()
}

// Counts 返回当前统计计数
func (m *ManagedBreaker[T]) Counts() Counts {
	return m.cb.Counts()
}

// CircuitBreaker 返回底层的泛型熔断器
func (m *ManagedBreaker[T]) CircuitBreaker() *gobreaker.CircuitBreaker[T] {
	return m.cb
}
