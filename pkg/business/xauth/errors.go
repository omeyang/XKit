package xauth

import (
	"errors"
	"fmt"
)

// =============================================================================
// 配置错误
// =============================================================================

var (
	// ErrNilClient 表示传入的 Client 为 nil。
	ErrNilClient = errors.New("xauth: nil client")

	// ErrNilConfig 表示传入的配置为 nil。
	ErrNilConfig = errors.New("xauth: nil config")

	// ErrMissingHost 表示认证服务地址未配置。
	ErrMissingHost = errors.New("xauth: missing host")

	// ErrInvalidTimeout 表示超时配置无效。
	ErrInvalidTimeout = errors.New("xauth: invalid timeout")

	// ErrInvalidRefreshThreshold 表示 Token 刷新阈值无效。
	ErrInvalidRefreshThreshold = errors.New("xauth: invalid refresh threshold")

	// ErrInsecureHost 表示 Host 使用了非 HTTPS 协议。
	// 认证服务传输 Bearer Token 和客户端凭据，明文 HTTP 会暴露敏感信息。
	// 如需在开发/测试环境中使用 HTTP，请设置 Config.AllowInsecure = true。
	ErrInsecureHost = errors.New("xauth: host must use https:// (set AllowInsecure=true for development)")

	// ErrInvalidHost 表示 Host 格式无效。
	// Host 必须包含协议和主机名，例如 "https://auth.example.com"。
	ErrInvalidHost = errors.New("xauth: invalid host: must include scheme and host (e.g., https://auth.example.com)")

	// ErrNilRedisClient 表示 Redis 客户端为 nil。
	ErrNilRedisClient = errors.New("xauth: nil redis client")

	// ErrNilHTTPClient 表示 HTTP 客户端为 nil。
	ErrNilHTTPClient = errors.New("xauth: nil http client")

	// ErrNilCache 表示缓存为 nil。
	ErrNilCache = errors.New("xauth: nil cache")
)

// =============================================================================
// 认证错误
// =============================================================================

var (
	// ErrNilRequest 表示传入的请求为 nil。
	ErrNilRequest = errors.New("xauth: nil request")

	// ErrMissingTenantID 表示租户 ID 未提供。
	ErrMissingTenantID = errors.New("xauth: missing tenant_id")

	// ErrMissingToken 表示 Token 未提供。
	ErrMissingToken = errors.New("xauth: missing token")

	// ErrMissingAPIKey 表示 API Key 未配置。
	ErrMissingAPIKey = errors.New("xauth: missing api_key")
)

// =============================================================================
// Token 错误
// =============================================================================

var (
	// ErrTokenNotFound 表示缓存中未找到 Token。
	ErrTokenNotFound = errors.New("xauth: token not found")

	// ErrTokenInvalid 表示 Token 无效（验证失败）。
	ErrTokenInvalid = errors.New("xauth: token invalid")

	// ErrRefreshTokenNotFound 表示 Refresh Token 未找到。
	ErrRefreshTokenNotFound = errors.New("xauth: refresh token not found")
)

// =============================================================================
// 请求错误
// =============================================================================

var (
	// ErrRequestFailed 表示 HTTP 请求失败。
	ErrRequestFailed = errors.New("xauth: request failed")

	// ErrResponseInvalid 表示响应格式无效。
	ErrResponseInvalid = errors.New("xauth: invalid response")

	// ErrResponseTooLarge 表示响应体超过最大限制。
	// 默认限制为 10MB，超过此限制的响应会被拒绝而非截断。
	ErrResponseTooLarge = errors.New("xauth: response body exceeds maximum size limit")

	// ErrUnauthorized 表示认证失败（401）。
	ErrUnauthorized = errors.New("xauth: unauthorized")

	// ErrForbidden 表示权限不足（403）。
	ErrForbidden = errors.New("xauth: forbidden")

	// ErrNotFound 表示资源不存在（404）。
	ErrNotFound = errors.New("xauth: not found")

	// ErrServerError 表示服务端错误（5xx）。
	ErrServerError = errors.New("xauth: server error")
)

// =============================================================================
// 平台信息错误
// =============================================================================

var (
	// ErrPlatformIDNotFound 表示平台 ID 未找到。
	ErrPlatformIDNotFound = errors.New("xauth: platform_id not found")

	// ErrUnclassRegionIDNotFound 表示未归类组 Region ID 未找到。
	ErrUnclassRegionIDNotFound = errors.New("xauth: unclass_region_id not found")
)

// =============================================================================
// 缓存错误
// =============================================================================

var (
	// ErrCacheMiss 表示缓存未命中。
	ErrCacheMiss = errors.New("xauth: cache miss")
)

// =============================================================================
// 客户端状态错误
// =============================================================================

var (
	// ErrClientClosed 表示客户端已关闭。
	ErrClientClosed = errors.New("xauth: client closed")
)

// =============================================================================
// 可重试错误包装
// =============================================================================

// RetryableError 可重试错误接口。
// 实现此接口的错误会被自动识别为可重试或不可重试。
type RetryableError interface {
	error
	Retryable() bool
}

// TemporaryError 临时性错误（应该重试）。
type TemporaryError struct {
	Err error
}

// NewTemporaryError 创建临时性错误。
func NewTemporaryError(err error) *TemporaryError {
	return &TemporaryError{Err: err}
}

func (e *TemporaryError) Error() string {
	if e.Err == nil {
		return "xauth: temporary error"
	}
	return e.Err.Error()
}

func (e *TemporaryError) Unwrap() error {
	return e.Err
}

func (e *TemporaryError) Retryable() bool {
	return true
}

// PermanentError 永久性错误（不应重试）。
type PermanentError struct {
	Err error
}

// NewPermanentError 创建永久性错误。
func NewPermanentError(err error) *PermanentError {
	return &PermanentError{Err: err}
}

func (e *PermanentError) Error() string {
	if e.Err == nil {
		return "xauth: permanent error"
	}
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

func (e *PermanentError) Retryable() bool {
	return false
}

// IsRetryable 检查错误是否可重试。
// 设计决策: 重试基础设施（IsRetryable/RetryableError/TemporaryError/PermanentError）
// 是提供给调用方使用的构建块——调用方根据自身场景决定重试策略（最大次数、退避算法等）。
// 库内部仅实现 401 自动重试（见 EnableAutoRetryOn401），不做通用自动重试，
// 避免在不同业务场景下产生不合适的重试行为。
//
// 规则：
//   - nil 错误：不需要重试（视为成功）
//   - 实现 RetryableError 接口：根据 Retryable() 返回值判断
//   - ErrServerError：可重试
//   - ErrRequestFailed：可重试
//   - 其他错误：默认不可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var re RetryableError
	if errors.As(err, &re) {
		return re.Retryable()
	}

	// 特定错误类型的可重试判断
	if errors.Is(err, ErrServerError) || errors.Is(err, ErrRequestFailed) {
		return true
	}

	// 默认：不可重试
	return false
}

// IsPermanent 检查错误是否为永久性错误。
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	return !IsRetryable(err)
}

// =============================================================================
// API 错误包装
// =============================================================================

// APIError 表示 API 返回的错误。
type APIError struct {
	StatusCode int
	Code       int
	Message    string
	Err        error
}

// NewAPIError 创建 API 错误。
func NewAPIError(statusCode, code int, message string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("xauth: api error: status=%d, code=%d, message=%s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("xauth: api error: status=%d, code=%d", e.StatusCode, e.Code)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// Retryable 判断 API 错误是否可重试。
// 5xx 错误视为可重试，4xx 错误视为不可重试。
func (e *APIError) Retryable() bool {
	return e.StatusCode >= 500
}

// Is 实现 errors.Is 接口。
// 设计决策: 使用直接 == 比较而非 errors.Is，因为 target 参数是调用方传入的哨兵错误，
// 而 ErrUnauthorized 等均为 errors.New 创建的简单值，无需递归 Unwrap。
func (e *APIError) Is(target error) bool {
	switch {
	case e.StatusCode == 401:
		return target == ErrUnauthorized
	case e.StatusCode == 403:
		return target == ErrForbidden
	case e.StatusCode == 404:
		return target == ErrNotFound
	case e.StatusCode >= 500:
		return target == ErrServerError
	}
	return false
}
