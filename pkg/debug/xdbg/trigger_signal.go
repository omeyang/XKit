//go:build !windows

package xdbg

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalTrigger 信号触发器。
// 监听 SIGUSR1 信号，收到信号时触发 Toggle 事件。
type SignalTrigger struct {
	sigCh     chan os.Signal
	closeOnce sync.Once
}

// NewSignalTrigger 创建信号触发器。
func NewSignalTrigger() *SignalTrigger {
	return &SignalTrigger{
		sigCh: make(chan os.Signal, 1),
	}
}

// Watch 开始监听信号。
func (t *SignalTrigger) Watch(ctx context.Context) <-chan TriggerEvent {
	eventCh := make(chan TriggerEvent, 1)

	// 注册信号处理
	signal.Notify(t.sigCh, syscall.SIGUSR1)

	go func() {
		defer close(eventCh)
		defer signal.Stop(t.sigCh)

		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-t.sigCh:
				if !ok {
					return
				}
				if sig == syscall.SIGUSR1 {
					select {
					case eventCh <- TriggerEventToggle:
					default:
						// 通道已满，跳过
					}
				}
			}
		}
	}()

	return eventCh
}

// Close 关闭触发器。
func (t *SignalTrigger) Close() error {
	t.closeOnce.Do(func() {
		signal.Stop(t.sigCh)
		close(t.sigCh)
	})
	return nil
}
