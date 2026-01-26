//go:build !windows

package xdbg

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestTriggerEvent_String(t *testing.T) {
	tests := []struct {
		event TriggerEvent
		want  string
	}{
		{TriggerEventEnable, "Enable"},
		{TriggerEventDisable, "Disable"},
		{TriggerEventToggle, "Toggle"},
		{TriggerEvent(0), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.event.String(); got != tt.want {
				t.Errorf("TriggerEvent.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignalTrigger(t *testing.T) {
	trigger := NewSignalTrigger()
	//nolint:errcheck // test cleanup: 测试触发器关闭失败不影响测试结果
	defer func() { _ = trigger.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh := trigger.Watch(ctx)

	// 发送 SIGUSR1 信号给自己
	go func() {
		time.Sleep(100 * time.Millisecond)
		//nolint:errcheck // test trigger: 向自身发送信号触发测试，失败会导致测试超时而非静默失败
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	}()

	select {
	case event := <-eventCh:
		if event != TriggerEventToggle {
			t.Errorf("expected TriggerEventToggle, got %v", event)
		}
	case <-ctx.Done():
		t.Error("timeout waiting for trigger event")
	}
}

func TestSignalTrigger_ContextCancel(t *testing.T) {
	trigger := NewSignalTrigger()
	//nolint:errcheck // test cleanup: 测试触发器关闭失败不影响测试结果
	defer func() { _ = trigger.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	eventCh := trigger.Watch(ctx)

	// 立即取消
	cancel()

	// 等待通道关闭
	select {
	case _, ok := <-eventCh:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for channel to close")
	}
}

func TestSignalTrigger_Close(t *testing.T) {
	trigger := NewSignalTrigger()

	// 第一次关闭应该成功
	if err := trigger.Close(); err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// 第二次关闭也应该成功（幂等）
	if err := trigger.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}
