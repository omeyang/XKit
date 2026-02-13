package xretry

import (
	"context"
	"errors"
)

// RetryableError 可重试错误接口
// 实现此接口的错误会被自动识别为可重试或不可重试
type RetryableError interface {
	error
	Retryable() bool
}

// PermanentError 永久性错误（不应重试）
type PermanentError struct {
	Err error
}

// NewPermanentError 创建永久性错误
func NewPermanentError(err error) *PermanentError {
	return &PermanentError{Err: err}
}

func (e *PermanentError) Error() string {
	if e.Err == nil {
		return "permanent error"
	}
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

func (e *PermanentError) Retryable() bool {
	return false
}

// TemporaryError 临时性错误（应该重试）
type TemporaryError struct {
	Err error
}

// NewTemporaryError 创建临时性错误
func NewTemporaryError(err error) *TemporaryError {
	return &TemporaryError{Err: err}
}

func (e *TemporaryError) Error() string {
	if e.Err == nil {
		return "temporary error"
	}
	return e.Err.Error()
}

func (e *TemporaryError) Unwrap() error {
	return e.Err
}

func (e *TemporaryError) Retryable() bool {
	return true
}

// IsRetryable 检查错误是否可重试
// 规则：
//   - nil 错误：不需要重试（视为成功）
//   - context.Canceled / context.DeadlineExceeded：不可重试（fail-fast）
//   - 实现 RetryableError 接口：根据 Retryable() 返回值判断
//   - 其他错误：默认视为可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 设计决策: context 取消类错误默认不可重试。即使函数返回的是内部子 context 的
	// 取消错误（而非传入 Do 的外层 context），也应 fail-fast 避免无效重试。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var re RetryableError
	if errors.As(err, &re) {
		return re.Retryable()
	}

	// 默认：未知错误视为可重试
	return true
}

// IsPermanent 检查错误是否为永久性错误
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	return !IsRetryable(err)
}
