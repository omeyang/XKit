package xdbg

import "context"

// TriggerEvent 触发事件类型。
type TriggerEvent int

const (
	// TriggerEventEnable 启用调试服务。
	TriggerEventEnable TriggerEvent = iota + 1

	// TriggerEventDisable 禁用调试服务。
	TriggerEventDisable

	// TriggerEventToggle 切换调试服务状态（开关模式）。
	TriggerEventToggle
)

// String 返回触发事件的字符串表示。
func (e TriggerEvent) String() string {
	switch e {
	case TriggerEventEnable:
		return "Enable"
	case TriggerEventDisable:
		return "Disable"
	case TriggerEventToggle:
		return "Toggle"
	default:
		return "Unknown"
	}
}

// Trigger 触发器接口。
// 触发器负责监听外部事件（如信号），并通知调试服务启停。
type Trigger interface {
	// Watch 开始监听触发事件。
	// 返回一个事件通道，当触发时发送事件。
	// 当 ctx 被取消时，应关闭通道并返回。
	Watch(ctx context.Context) <-chan TriggerEvent

	// Close 关闭触发器，释放资源。
	Close() error
}
