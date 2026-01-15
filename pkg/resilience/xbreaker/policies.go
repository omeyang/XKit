package xbreaker

// ConsecutiveFailuresPolicy 连续失败熔断策略
//
// 当连续失败次数达到阈值时触发熔断。
// 这是最常用的熔断策略，适用于大多数场景。
type ConsecutiveFailuresPolicy struct {
	threshold uint32
}

// NewConsecutiveFailures 创建连续失败熔断策略
//
// threshold: 触发熔断的连续失败次数
//
// 示例:
//
//	policy := xbreaker.NewConsecutiveFailures(5)
//	// 连续失败 5 次后触发熔断
func NewConsecutiveFailures(threshold uint32) *ConsecutiveFailuresPolicy {
	return &ConsecutiveFailuresPolicy{
		threshold: threshold,
	}
}

// ReadyToTrip 判断是否应该触发熔断
func (p *ConsecutiveFailuresPolicy) ReadyToTrip(counts Counts) bool {
	return counts.ConsecutiveFailures >= p.threshold
}

// Threshold 返回阈值
func (p *ConsecutiveFailuresPolicy) Threshold() uint32 {
	return p.threshold
}

// FailureRatioPolicy 失败率熔断策略
//
// 当失败率超过阈值时触发熔断。
// 只有当请求数达到最小请求数时才会计算失败率。
type FailureRatioPolicy struct {
	ratio       float64 // 失败率阈值 (0.0 - 1.0)
	minRequests uint32  // 最小请求数
}

// NewFailureRatio 创建失败率熔断策略
//
// ratio: 失败率阈值 (0.0 - 1.0)，例如 0.5 表示 50% 失败率
// minRequests: 最小请求数，请求数不足时不触发熔断
//
// 示例:
//
//	policy := xbreaker.NewFailureRatio(0.5, 10)
//	// 失败率超过 50% 且请求数 >= 10 时触发熔断
func NewFailureRatio(ratio float64, minRequests uint32) *FailureRatioPolicy {
	// 确保 ratio 在有效范围内
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return &FailureRatioPolicy{
		ratio:       ratio,
		minRequests: minRequests,
	}
}

// ReadyToTrip 判断是否应该触发熔断
func (p *FailureRatioPolicy) ReadyToTrip(counts Counts) bool {
	// 请求数不足或为零，不触发熔断（避免除零）
	if counts.Requests == 0 || counts.Requests < p.minRequests {
		return false
	}

	// 计算失败率
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return failureRatio >= p.ratio
}

// Ratio 返回失败率阈值
func (p *FailureRatioPolicy) Ratio() float64 {
	return p.ratio
}

// MinRequests 返回最小请求数
func (p *FailureRatioPolicy) MinRequests() uint32 {
	return p.minRequests
}

// FailureCountPolicy 失败次数熔断策略
//
// 当失败次数达到阈值时触发熔断。
// 与 ConsecutiveFailuresPolicy 不同，这里统计的是总失败次数，不要求连续。
type FailureCountPolicy struct {
	threshold uint32
}

// NewFailureCount 创建失败次数熔断策略
//
// threshold: 触发熔断的失败次数
//
// 示例:
//
//	policy := xbreaker.NewFailureCount(10)
//	// 统计窗口内失败 10 次后触发熔断
func NewFailureCount(threshold uint32) *FailureCountPolicy {
	return &FailureCountPolicy{
		threshold: threshold,
	}
}

// ReadyToTrip 判断是否应该触发熔断
func (p *FailureCountPolicy) ReadyToTrip(counts Counts) bool {
	return counts.TotalFailures >= p.threshold
}

// Threshold 返回阈值
func (p *FailureCountPolicy) Threshold() uint32 {
	return p.threshold
}

// CompositePolicy 组合熔断策略
//
// 组合多个策略，任一策略满足即触发熔断。
// 适用于需要多重熔断条件的场景。
type CompositePolicy struct {
	policies []TripPolicy
}

// NewCompositePolicy 创建组合熔断策略
//
// 传入的 nil 策略会被自动过滤。
//
// 示例:
//
//	policy := xbreaker.NewCompositePolicy(
//	    xbreaker.NewConsecutiveFailures(5),
//	    xbreaker.NewFailureRatio(0.5, 10),
//	)
//	// 连续失败 5 次 OR 失败率超过 50% 时触发熔断
func NewCompositePolicy(policies ...TripPolicy) *CompositePolicy {
	// 过滤 nil 策略
	filtered := make([]TripPolicy, 0, len(policies))
	for _, p := range policies {
		if p != nil {
			filtered = append(filtered, p)
		}
	}
	return &CompositePolicy{
		policies: filtered,
	}
}

// ReadyToTrip 判断是否应该触发熔断
// 任一子策略返回 true 即触发熔断
func (p *CompositePolicy) ReadyToTrip(counts Counts) bool {
	for _, policy := range p.policies {
		if policy.ReadyToTrip(counts) {
			return true
		}
	}
	return false
}

// Policies 返回所有子策略的副本
//
// 返回副本以防止外部修改内部状态。
func (p *CompositePolicy) Policies() []TripPolicy {
	if len(p.policies) == 0 {
		return nil
	}
	// 返回副本，防止外部修改
	result := make([]TripPolicy, len(p.policies))
	copy(result, p.policies)
	return result
}

// NeverTripPolicy 永不熔断策略
//
// 用于测试或特殊场景，熔断器永远不会打开。
type NeverTripPolicy struct{}

// NewNeverTrip 创建永不熔断策略
func NewNeverTrip() *NeverTripPolicy {
	return &NeverTripPolicy{}
}

// ReadyToTrip 永远返回 false
func (p *NeverTripPolicy) ReadyToTrip(_ Counts) bool {
	return false
}

// AlwaysTripPolicy 总是熔断策略
//
// 用于测试场景，任何失败都会触发熔断。
type AlwaysTripPolicy struct{}

// NewAlwaysTrip 创建总是熔断策略
func NewAlwaysTrip() *AlwaysTripPolicy {
	return &AlwaysTripPolicy{}
}

// ReadyToTrip 只要有失败就返回 true
func (p *AlwaysTripPolicy) ReadyToTrip(counts Counts) bool {
	return counts.TotalFailures > 0
}

// SlowCallRatioPolicy 慢调用熔断策略（基于失败率的近似实现）
//
// 重要说明：此策略本质上是 FailureRatioPolicy 的别名实现。
// gobreaker 原生不支持慢调用时间统计，因此需要用户自行实现慢调用检测：
//
//  1. 创建 Breaker 时通过 WithSuccessPolicy 设置自定义成功判定策略
//  2. 在 SuccessPolicy.IsSuccessful 中检查调用耗时，超过阈值时返回 false（标记为失败）
//  3. 使用此策略基于"失败率"来近似"慢调用率"
//
// 示例:
//
//	// 方式一：实现 SuccessPolicy 接口
//	type slowCallChecker struct{}
//
//	func (s *slowCallChecker) IsSuccessful(err error) bool {
//	    if errors.Is(err, ErrSlowCall) {
//	        return false // 慢调用视为失败
//	    }
//	    return err == nil
//	}
//
//	breaker := xbreaker.NewBreaker("slow-api",
//	    xbreaker.WithTripPolicy(xbreaker.NewSlowCallRatio(0.5, 10)),
//	    xbreaker.WithSuccessPolicy(&slowCallChecker{}),
//	)
//
//	// 方式二：直接使用 gobreaker 原生 API（更灵活）
//	cb := xbreaker.NewCircuitBreaker[string](xbreaker.Settings{
//	    Name: "slow-api",
//	    ReadyToTrip: xbreaker.NewSlowCallRatio(0.5, 10).ReadyToTrip,
//	    IsSuccessful: func(err error) bool {
//	        return !errors.Is(err, ErrSlowCall) && err == nil
//	    },
//	})
//
// 如果不需要慢调用检测，建议直接使用 FailureRatioPolicy，语义更清晰。
type SlowCallRatioPolicy struct {
	ratio       float64
	minRequests uint32
}

// NewSlowCallRatio 创建慢调用熔断策略
//
// ratio: 阈值 (0.0 - 1.0)，超过此比例时触发熔断
// minRequests: 最小请求数，请求数不足时不触发熔断
//
// 重要：此策略统计的是"失败率"，需要配合 IsSuccessful 将慢调用标记为失败。
// 详见 SlowCallRatioPolicy 类型文档。
func NewSlowCallRatio(ratio float64, minRequests uint32) *SlowCallRatioPolicy {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return &SlowCallRatioPolicy{
		ratio:       ratio,
		minRequests: minRequests,
	}
}

// ReadyToTrip 判断是否应该触发熔断
func (p *SlowCallRatioPolicy) ReadyToTrip(counts Counts) bool {
	// 请求数不足或为零，不触发熔断（避免除零）
	if counts.Requests == 0 || counts.Requests < p.minRequests {
		return false
	}
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return failureRatio >= p.ratio
}

// Ratio 返回慢调用率阈值
func (p *SlowCallRatioPolicy) Ratio() float64 {
	return p.ratio
}

// MinRequests 返回最小请求数
func (p *SlowCallRatioPolicy) MinRequests() uint32 {
	return p.minRequests
}
