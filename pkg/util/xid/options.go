package xid

import "time"

// =============================================================================
// 配置
// =============================================================================

// options 内部配置结构
type options struct {
	machineID        func() (uint16, error)
	checkMachineID   func(uint16) bool
	maxWaitDuration  time.Duration
	maxWaitSet       bool // 区分"未传入"与"显式传入 0"
	retryInterval    time.Duration
	retryIntervalSet bool // 区分"未传入"与"显式传入 0"
}

// Option 配置选项函数
type Option func(*options)

// WithMachineID 设置自定义机器 ID 生成函数。
//
// 默认使用 [DefaultMachineID] 的多层回退策略（环境变量 → Pod 名称哈希 →
// 主机名哈希 → 私有 IP 低 16 位），详见 [DefaultMachineID] 文档。
// 在以下场景可能需要自定义：
//   - 需要与外部服务协调机器 ID 分配（如 etcd/ZooKeeper 注册）
//   - 需要基于自定义信息确定机器 ID
//
// 函数返回的 ID 必须在 0-65535 范围内。
func WithMachineID(fn func() (uint16, error)) Option {
	return func(c *options) {
		c.machineID = fn
	}
}

// WithCheckMachineID 设置机器 ID 验证函数。
//
// 在创建生成器时会调用此函数验证机器 ID 的有效性。
// 如果返回 false，Init 会失败。
//
// 典型用途：
//   - 检查机器 ID 是否在预期范围内
//   - 通过外部服务验证机器 ID 未被占用
func WithCheckMachineID(fn func(uint16) bool) Option {
	return func(c *options) {
		c.checkMachineID = fn
	}
}

// WithMaxWaitDuration 设置时钟回拨时的最大等待时间。
//
// 当使用 NewWithRetry 等方法时，
// 如果检测到时钟回拨，会等待一段时间让时钟追上。
// 此选项设置最大等待时间，超过后返回 ErrClockBackwardTimeout。
//
// 默认值为 500ms，适合大多数 NTP 同步场景。
// 如果你的环境时钟漂移较大，可以适当增加。
// 传入负值会在 NewGenerator 中返回错误（fail-fast）。
// 传入零值表示"不等待"，即 NewWithRetry 在首次失败后立即返回超时错误。
// 不调用此选项则使用默认值 500ms。
func WithMaxWaitDuration(d time.Duration) Option {
	return func(c *options) {
		c.maxWaitDuration = d
		c.maxWaitSet = true
	}
}

// WithRetryInterval 设置时钟回拨等待时的重试间隔。
//
// 当检测到时钟回拨时，每隔此间隔尝试一次生成 ID。
// 默认值为 10ms（sonyflake 时间精度是 10ms）。
// 传入负值会在 NewGenerator 中返回错误（fail-fast）。
// 传入零值表示"无间隔"，即重试不等待直接重新尝试。
// 不调用此选项则使用默认值 10ms。
func WithRetryInterval(d time.Duration) Option {
	return func(c *options) {
		c.retryInterval = d
		c.retryIntervalSet = true
	}
}
