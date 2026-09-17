//go:build windows

package xdbg

import (
	"context"
	"errors"
)

// ErrSignalNotSupported 表示信号触发在 Windows 上不支持。
var ErrSignalNotSupported = errors.New("xdbg: signal trigger is not supported on Windows")

// SignalTrigger Windows 平台的信号触发器（不支持）。
type SignalTrigger struct{}

// NewSignalTrigger 创建信号触发器（Windows 不支持）。
func NewSignalTrigger() *SignalTrigger {
	return &SignalTrigger{}
}

// Watch 在 Windows 上不支持，立即返回空通道。
func (t *SignalTrigger) Watch(_ context.Context) <-chan TriggerEvent {
	ch := make(chan TriggerEvent)
	close(ch)
	return ch
}

// Close 关闭触发器。
func (t *SignalTrigger) Close() error {
	return nil
}
