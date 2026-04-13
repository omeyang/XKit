package xlimit

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

// Result 限流检查结果
type Result struct {
	// Allowed 是否允许请求通过
	Allowed bool

	// Limit 当前规则的配额上限
	Limit int

	// Remaining 当前窗口内剩余配额
	Remaining int

	// ResetAt 配额重置时间
	ResetAt time.Time

	// RetryAfter 建议重试等待时间（仅在 Allowed=false 时有意义）
	RetryAfter time.Duration

	// Rule 触发限流的规则名称
	Rule string

	// Key 触发限流的键
	Key string
}

// Headers 返回标准限流响应头
// - X-RateLimit-Limit: 配额上限
// - X-RateLimit-Remaining: 剩余配额
// - X-RateLimit-Reset: 配额重置时间（Unix 时间戳）
// - Retry-After: 重试等待秒数（仅在被限流时，向上取整确保最小值为 1）
func (r *Result) Headers() map[string]string {
	headers := map[string]string{
		"X-RateLimit-Limit":     strconv.Itoa(r.Limit),
		"X-RateLimit-Remaining": strconv.Itoa(r.Remaining),
		"X-RateLimit-Reset":     strconv.FormatInt(r.ResetAt.Unix(), 10),
	}

	if r.RetryAfter > 0 {
		// 设计决策: 使用 math.Ceil 向上取整，避免亚秒级等待被截断为 0，
		// 导致客户端立即重试并放大瞬时流量。
		retryAfterSec := int64(math.Ceil(r.RetryAfter.Seconds()))
		headers["Retry-After"] = strconv.FormatInt(retryAfterSec, 10)
	}

	return headers
}

// SetHeaders 将限流响应头写入 http.ResponseWriter
//
// 设计决策: 当 Limit <= 0 时跳过写入配额头。
// Limit=0 表示无有效配额信息（如 FallbackOpen 或无匹配规则），
// 写入 X-RateLimit-Limit: 0 会误导客户端认为配额为零。
func (r *Result) SetHeaders(w http.ResponseWriter) {
	if r.Limit <= 0 {
		return
	}
	for key, value := range r.Headers() {
		w.Header().Set(key, value)
	}
}

// AllowedResult 创建一个允许通过的结果
func AllowedResult(limit, remaining int) *Result {
	return &Result{
		Allowed:   true,
		Limit:     limit,
		Remaining: remaining,
	}
}

// DeniedResult 创建一个被拒绝的结果
func DeniedResult(limit int, retryAfter time.Duration, rule, key string) *Result {
	return &Result{
		Allowed:    false,
		Limit:      limit,
		Remaining:  0,
		RetryAfter: retryAfter,
		Rule:       rule,
		Key:        key,
	}
}
