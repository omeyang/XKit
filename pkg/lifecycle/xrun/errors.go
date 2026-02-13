package xrun

import (
	"errors"
	"fmt"
	"os"
)

// ErrSignal 表示因收到系统信号而终止。
// 使用 errors.Is(err, ErrSignal) 判断是否为信号错误。
var ErrSignal = errors.New("received signal")

// ErrInvalidInterval 表示 Ticker 的间隔参数无效（必须为正数）。
var ErrInvalidInterval = errors.New("xrun: interval must be positive")

// ErrInvalidDelay 表示 Timer 的延迟参数无效（不能为负数）。
var ErrInvalidDelay = errors.New("xrun: delay must not be negative")

// SignalError 包含触发终止的具体信号信息。
//
// Run/RunServices/RunWithOptions 在收到系统信号时返回此错误。
// 使用 errors.Is(err, ErrSignal) 判断是否为信号错误，
// 使用 errors.As 获取具体信号值：
//
//	var sigErr *xrun.SignalError
//	if errors.As(err, &sigErr) {
//	    fmt.Printf("received signal: %v\n", sigErr.Signal)
//	}
type SignalError struct {
	Signal os.Signal
}

// Error 实现 error 接口。
func (e *SignalError) Error() string {
	if e.Signal == nil {
		return "received signal <nil>"
	}
	return fmt.Sprintf("received signal %s", e.Signal)
}

// Is 支持 errors.Is(err, ErrSignal) 判断。
func (e *SignalError) Is(target error) bool {
	return target == ErrSignal
}

// Unwrap 返回底层错误。
func (e *SignalError) Unwrap() error {
	return ErrSignal
}
