package xlimit

import (
	"context"
	"time"
)

// CheckResult 后端检查结果
type CheckResult struct {
	Allowed    bool          // 是否允许
	Limit      int           // 实际使用的配额上限（本地后端可能会调整）
	Remaining  int           // 剩余配额
	ResetAt    time.Time     // 配额重置时间
	RetryAfter time.Duration // 如果被限流，建议重试等待时间
}

// Backend 定义限流后端的核心操作接口
// 职责单一：只负责底层的限流检查，不包含可观测性、回调等关注点
// 实现应该是并发安全的
type Backend interface {
	// CheckRule 检查单个规则是否允许请求通过
	// 参数:
	//   - ctx: 上下文
	//   - key: 渲染后的限流键（如 "tenant:acme:api:POST:/orders"）
	//   - limit: 配额上限
	//   - burst: 突发容量
	//   - window: 时间窗口
	//   - n: 请求数量
	//
	// 返回 CheckResult 和可能的错误
	CheckRule(ctx context.Context, key string, limit, burst int, window time.Duration, n int) (CheckResult, error)

	// Reset 重置指定键的限流计数
	Reset(ctx context.Context, key string) error

	// Query 查询当前配额状态（不消耗配额）
	// effectiveLimit 为后端实际生效的 limit（本地后端会按 podCount 调整）
	Query(ctx context.Context, key string, limit, burst int, window time.Duration) (
		effectiveLimit, remaining int, resetAt time.Time, err error)

	// Close 释放后端自有资源（不关闭注入的外部客户端）
	// 设计决策: 保留 ctx 参数（D-02），当前未使用但预留用于未来超时控制。
	Close(ctx context.Context) error

	// Type 返回后端类型标识，用于日志和指标
	Type() string
}
