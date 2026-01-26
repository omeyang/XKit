package xlimit

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

// =============================================================================
// 预定义错误
// =============================================================================

// 预定义错误，使用 errors.Is 进行比较
var (
	// ErrRateLimited 表示请求被限流
	ErrRateLimited = errors.New("xlimit: rate limited")

	// ErrRedisUnavailable 表示 Redis 不可用
	ErrRedisUnavailable = errors.New("xlimit: redis unavailable")

	// ErrInvalidRule 表示限流规则无效
	ErrInvalidRule = errors.New("xlimit: invalid rule")

	// ErrInvalidKey 表示限流键无效
	ErrInvalidKey = errors.New("xlimit: invalid key")

	// ErrLimiterClosed 表示限流器已关闭
	ErrLimiterClosed = errors.New("xlimit: limiter closed")

	// ErrConfigNotFound 表示配置文件不存在
	ErrConfigNotFound = errors.New("xlimit: config not found")

	// ErrNoRuleMatched 表示没有匹配的规则
	ErrNoRuleMatched = errors.New("xlimit: no rule matched")
)

// =============================================================================
// 限流错误类型
// =============================================================================

// LimitError 限流错误
//
// 包含限流的详细信息，参考 xbreaker 的设计。
// 实现了 error 接口和 errors.Is 支持。
type LimitError struct {
	// Key 被限流的键
	Key Key
	// Rule 触发限流的规则名称
	Rule string
	// Limit 配额上限
	Limit int
	// Remaining 剩余配额
	Remaining int
	// Reason 限流原因（可选）
	Reason string
}

// Error 实现 error 接口
func (e *LimitError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("xlimit: rate limited by rule %q, key=%s, limit=%d, remaining=%d, reason=%s",
			e.Rule, e.Key.String(), e.Limit, e.Remaining, e.Reason)
	}
	return fmt.Sprintf("xlimit: rate limited by rule %q, key=%s, limit=%d, remaining=%d",
		e.Rule, e.Key.String(), e.Limit, e.Remaining)
}

// Is 支持 errors.Is 检查
func (e *LimitError) Is(target error) bool {
	return target == ErrRateLimited
}

// Unwrap 返回底层错误
func (e *LimitError) Unwrap() error {
	return ErrRateLimited
}

// Retryable 返回是否可重试
//
// 限流错误通常不可重试，应等待配额重置。
func (e *LimitError) Retryable() bool {
	return false
}

// =============================================================================
// 错误检查函数
// =============================================================================

// IsDenied 检查错误是否为限流错误
//
// 支持 LimitError。
func IsDenied(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsRetryable 检查错误是否可重试
//
// 限流错误不可重试，Redis 错误可重试。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// 限流错误不可重试
	if IsDenied(err) {
		return false
	}
	// Redis 错误可重试
	return IsRedisError(err)
}

// redisRelatedErrors 包含所有需要检查的 Redis 相关错误
var redisRelatedErrors = []error{
	ErrRedisUnavailable,
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
	syscall.EPIPE,
	syscall.ETIMEDOUT,
	io.EOF,
	io.ErrUnexpectedEOF,
}

// IsRedisError 检查是否是 Redis 相关错误
//
// 使用类型断言和错误链检查，而不是字符串匹配。
func IsRedisError(err error) bool {
	if err == nil {
		return false
	}

	// 检查已知的错误类型
	for _, target := range redisRelatedErrors {
		if errors.Is(err, target) {
			return true
		}
	}

	// 检查网络相关错误
	return isNetworkError(err)
}

// isNetworkError 检查是否是网络相关错误
func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
