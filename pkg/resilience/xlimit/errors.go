package xlimit

import (
	"errors"
	"fmt"
)

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
)

// RateLimitError 包含限流结果的详细错误
type RateLimitError struct {
	result *Result
}

// NewRateLimitError 创建限流错误
func NewRateLimitError(result *Result) *RateLimitError {
	return &RateLimitError{result: result}
}

// Error 实现 error 接口
func (e *RateLimitError) Error() string {
	if e.result == nil {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("xlimit: rate limited by rule %q, key=%s, remaining=%d",
		e.result.Rule, e.result.Key, e.result.Remaining)
}

// Is 支持 errors.Is 检查
func (e *RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}

// Unwrap 返回底层错误
func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// Result 返回限流结果
func (e *RateLimitError) Result() *Result {
	return e.result
}

// IsRateLimited 检查错误是否为限流错误
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}
