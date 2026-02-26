package xbreaker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// 默认配置常量
const (
	// DefaultConsecutiveFailures 默认连续失败触发阈值
	DefaultConsecutiveFailures uint32 = 5

	// DefaultTimeout 默认 Open→HalfOpen 超时时间
	DefaultTimeout = 60 * time.Second

	// DefaultMaxRequests 默认 HalfOpen 最大请求数
	DefaultMaxRequests uint32 = 1
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
//
// 注意：被 IsSuccessful 标记为成功的错误会计入成功计数。
// 如果需要从统计中完全排除某些错误（不影响成功/失败计数），
// 请使用 ExcludePolicy。
type SuccessPolicy interface {
	// IsSuccessful 判断操作是否成功
	IsSuccessful(err error) bool
}

// ExcludePolicy 错误排除策略接口（可选）
//
// 实现此接口可自定义哪些错误应被排除在熔断统计之外。
// 被排除的错误不会影响成功计数或失败计数。
//
// 适用于排除 context.Canceled、context.DeadlineExceeded 等
// 客户端侧取消错误，避免它们影响熔断判定。
//
// 与 SuccessPolicy 的区别：
//   - SuccessPolicy: 将错误标记为"成功"，计入成功计数
//   - ExcludePolicy: 将错误从统计中排除，不计入任何计数
type ExcludePolicy interface {
	// IsExcluded 判断错误是否应被排除在统计之外
	IsExcluded(err error) bool
}

// Breaker 熔断器执行器
//
// Breaker 封装了 gobreaker 的熔断逻辑，提供更友好的 API。
// 使用 TripPolicy 接口抽象熔断判定，便于策略替换和测试。
type Breaker struct {
	name          string
	tripPolicy    TripPolicy
	successPolicy SuccessPolicy
	excludePolicy ExcludePolicy
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
//
// 注意：被 IsSuccessful 标记为成功的错误会计入成功计数。
// 如果需要从统计中完全排除某些错误（不影响任何计数），
// 请使用 WithExcludePolicy。
func WithSuccessPolicy(p SuccessPolicy) BreakerOption {
	return func(b *Breaker) {
		if p != nil {
			b.successPolicy = p
		}
	}
}

// WithExcludePolicy 设置错误排除策略
//
// 被排除的错误不计入成功或失败统计，不影响熔断器状态判定。
// 适用于排除 context.Canceled、context.DeadlineExceeded 等客户端取消错误。
//
// 与 WithSuccessPolicy 的区别：
//   - WithSuccessPolicy: 将错误标记为"成功"，计入成功计数
//   - WithExcludePolicy: 将错误从统计中排除，不计入任何计数
func WithExcludePolicy(p ExcludePolicy) BreakerOption {
	return func(b *Breaker) {
		if p != nil {
			b.excludePolicy = p
		}
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
		if d >= 0 {
			b.interval = d
		}
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
//
// 设计决策: 回调通过 goroutine 异步执行，避免与 gobreaker 内部 mutex 产生死锁。
// gobreaker 在 setState 方法中持有 sync.Mutex 期间调用 OnStateChange 回调，
// 若回调同步调用同一 Breaker 的 State()/Counts()/Do() 方法会导致不可恢复的死锁。
// 异步执行的代价是回调可能在状态变化后稍有延迟执行，且多次状态变化的回调顺序不保证。
//
// 注意：
//   - 回调异步执行，不阻塞熔断器状态转换
//   - 回调中的 panic 会被自动捕获并通过 slog.Error 记录，不会导致进程崩溃
//   - 多次快速状态变化时，回调执行顺序不保证
//   - 回调中获取的 State()/Counts() 可能已是更新后的值
func WithOnStateChange(f func(name string, from, to State)) BreakerOption {
	return func(b *Breaker) {
		if f != nil {
			b.onStateChange = f
		}
	}
}

// NewBreaker 创建熔断器执行器
//
// name 是熔断器的名称，用于日志和监控标识，建议传入非空字符串。
// 空名称不会报错，但会影响日志可读性和监控标签有效性。
//
// 默认配置：
//   - 熔断策略：连续失败 5 次触发熔断
//   - 超时时间：60 秒
//   - HalfOpen 最大请求数：1
func NewBreaker(name string, opts ...BreakerOption) *Breaker {
	b := &Breaker{
		name:        name,
		tripPolicy:  NewConsecutiveFailures(DefaultConsecutiveFailures),
		timeout:     DefaultTimeout,
		maxRequests: DefaultMaxRequests,
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
	// 校验 BucketPeriod 与 Interval 的一致性
	if b.bucketPeriod > 0 && b.interval == 0 {
		slog.Warn("xbreaker: WithBucketPeriod set without WithInterval, behavior depends on gobreaker internal defaults",
			"name", b.name, "bucketPeriod", b.bucketPeriod)
	}
	if b.bucketPeriod > 0 && b.interval > 0 && b.bucketPeriod > b.interval {
		slog.Warn("xbreaker: BucketPeriod exceeds Interval, sliding window may behave unexpectedly",
			"name", b.name, "interval", b.interval, "bucketPeriod", b.bucketPeriod)
	}

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

	// 如果有错误排除策略
	if b.excludePolicy != nil {
		st.IsExcluded = func(err error) bool {
			return b.excludePolicy.IsExcluded(err)
		}
	}

	// 设计决策: 回调通过 goroutine 异步执行，避免在 gobreaker 内部 mutex 持有期间
	// 同步调用回调导致死锁。详见 WithOnStateChange 文档。
	// goroutine 内使用 recover 隔离用户回调 panic，防止回调故障导致进程崩溃。
	if b.onStateChange != nil {
		cb := b.onStateChange
		st.OnStateChange = func(name string, from, to gobreaker.State) {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("xbreaker: OnStateChange callback panicked",
							"name", name, "from", from.String(), "to", to.String(), "panic", r)
					}
				}()
				cb(name, from, to)
			}()
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
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilFunc
	}
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
//   - b 不能为 nil，否则返回 ErrNilBreaker
//
// 设计决策: Execute 内部使用 CircuitBreaker[any] 导致返回值经过 interface boxing/unboxing。
// 对于高性能场景（热路径），推荐使用 ManagedBreaker[T] 避免此开销。
func Execute[T any](ctx context.Context, b *Breaker, fn func() (T, error)) (T, error) {
	var zero T

	if b == nil {
		return zero, ErrNilBreaker
	}
	if ctx == nil {
		return zero, ErrNilContext
	}
	if fn == nil {
		return zero, ErrNilFunc
	}

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
	// result 来自 fn()，类型始终为 T；nil 对应 T 的零值
	if result == nil {
		return zero, nil
	}
	// 设计决策: result 来自 fn() 返回的 T 类型值，类型断言理论上不可达失败路径；
	// 为安全起见返回错误而非静默返回零值，防止未来 gobreaker 内部变化导致数据丢失
	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("xbreaker: unexpected result type %T", result)
	}
	return typed, nil
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

// ExcludePolicy 返回当前错误排除策略
//
// 如果未设置排除策略，返回 nil
func (b *Breaker) ExcludePolicy() ExcludePolicy {
	return b.excludePolicy
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

// IsExcluded 判断错误是否应被排除在统计之外
//
// 如果设置了 ExcludePolicy 且 err 非 nil，使用它判断；否则返回 false。
func (b *Breaker) IsExcluded(err error) bool {
	if b.excludePolicy != nil && err != nil {
		return b.excludePolicy.IsExcluded(err)
	}
	return false
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
//
// 设计决策: ManagedBreaker 维护独立的熔断器状态，与传入的 Breaker 不共享计数和状态转换。
// 两者的 State()/Counts() 互不影响。这是因为 gobreaker 不支持跨类型参数共享状态，
// 传入的 Breaker 仅用于复用配置（TripPolicy、SuccessPolicy、Timeout 等）。
//
// 如果 b 为 nil，返回 ErrNilBreaker。
func NewManagedBreaker[T any](b *Breaker) (*ManagedBreaker[T], error) {
	if b == nil {
		return nil, ErrNilBreaker
	}

	// 复用 Breaker 的配置
	st := b.buildSettings()

	return &ManagedBreaker[T]{
		breaker: b,
		cb:      gobreaker.NewCircuitBreaker[T](st),
	}, nil
}

// Execute 执行受熔断器保护的操作
//
// 如果熔断器处于 Open 状态，操作不会被执行，直接返回 ErrOpenState。
// 如果熔断器处于 HalfOpen 状态且请求过多，返回 ErrTooManyRequests。
//
// 设计决策: Execute 不接受 context.Context 参数，以保持与 gobreaker 原生
// CircuitBreaker.Execute 签名一致。需要 context 取消支持时，
// 请在 fn 闭包中捕获 context 或使用 Execute[T] 包级函数。
//
// 注意：
//   - 熔断器错误会被包装为 BreakerError，实现 Retryable() 返回 false
//   - 这样在与 xretry 组合使用时，熔断错误不会被重试
func (m *ManagedBreaker[T]) Execute(fn func() (T, error)) (T, error) {
	if fn == nil {
		var zero T
		return zero, ErrNilFunc
	}
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
