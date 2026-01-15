package xetcd

import (
	"context"
	"fmt"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EventType 事件类型。
type EventType int

const (
	// EventPut 写入事件。
	EventPut EventType = iota
	// EventDelete 删除事件。
	EventDelete
)

// String 返回事件类型的字符串表示。
func (e EventType) String() string {
	switch e {
	case EventPut:
		return "PUT"
	case EventDelete:
		return "DELETE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", e)
	}
}

// Event Watch 事件。
type Event struct {
	// Type 事件类型。
	Type EventType

	// Key 键名。
	Key string

	// Value 键值。Delete 事件时为 nil。
	Value []byte

	// Revision 版本号。
	Revision int64

	// Error Watch 错误。
	// 非 nil 时表示 Watch 失败，其他字段无意义。
	// 接收到错误事件后，通道将被关闭，不会再有后续事件。
	Error error
}

// watchOptions Watch 选项。
type watchOptions struct {
	prefix   bool
	revision int64
}

// WatchOption Watch 选项函数。
type WatchOption func(*watchOptions)

// WithPrefix 启用前缀匹配模式，监听指定前缀下所有键的变化。
func WithPrefix() WatchOption {
	return func(o *watchOptions) {
		o.prefix = true
	}
}

// WithRevision 从指定版本开始 Watch，用于恢复 Watch 或获取历史事件。
func WithRevision(rev int64) WatchOption {
	return func(o *watchOptions) {
		o.revision = rev
	}
}

// Watch 监听键值变化，返回事件通道。
// 通过 context 取消监听，取消时关闭通道。
// 使用 WithPrefix() 监听前缀下所有键的变化。
//
// 事件处理：
//   - 普通事件：Event.Error 为 nil，其他字段有效
//   - 错误事件：Event.Error 非 nil，表示 Watch 失败，通道随后关闭
//
// 调用方应检查 Event.Error 以区分正常事件和 Watch 失败：
//
//	for event := range events {
//	    if event.Error != nil {
//	        // Watch 失败，处理错误
//	        log.Printf("watch error: %v", event.Error)
//	        return
//	    }
//	    // 处理正常事件
//	}
func (c *Client) Watch(ctx context.Context, key string, opts ...WatchOption) (<-chan Event, error) {
	if err := c.checkClosed(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	// 应用选项
	o := &watchOptions{}
	for _, opt := range opts {
		opt(o)
	}

	// 构建 etcd watch 选项
	etcdOpts := c.buildWatchOptions(o)

	// 创建事件通道
	eventCh := make(chan Event, 64)

	// 启动 watch goroutine
	go c.runWatchLoop(ctx, key, etcdOpts, eventCh)

	return eventCh, nil
}

// buildWatchOptions 构建 etcd watch 选项
func (c *Client) buildWatchOptions(o *watchOptions) []clientv3.OpOption {
	var etcdOpts []clientv3.OpOption
	if o.prefix {
		etcdOpts = append(etcdOpts, clientv3.WithPrefix())
	}
	if o.revision > 0 {
		etcdOpts = append(etcdOpts, clientv3.WithRev(o.revision))
	}
	return etcdOpts
}

// runWatchLoop 运行 watch 循环
func (c *Client) runWatchLoop(ctx context.Context, key string, etcdOpts []clientv3.OpOption, eventCh chan<- Event) {
	defer close(eventCh)

	watchCh := c.client.Watch(ctx, key, etcdOpts...)
	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-watchCh:
			if !ok {
				// watch 通道被关闭（通常是 context 取消导致）
				return
			}
			if resp.Err() != nil {
				// 发送错误事件，让调用方知道 Watch 失败
				c.sendErrorEvent(ctx, eventCh, resp.Err())
				return
			}
			if !c.dispatchEvents(ctx, resp.Events, eventCh) {
				return
			}
		}
	}
}

// sendErrorEvent 发送错误事件到通道
func (c *Client) sendErrorEvent(ctx context.Context, eventCh chan<- Event, err error) {
	select {
	case eventCh <- Event{Error: err}:
	case <-ctx.Done():
		// context 已取消，不发送错误事件
	}
}

// dispatchEvents 分发事件到通道，返回 false 表示应该退出循环
func (c *Client) dispatchEvents(ctx context.Context, events []*clientv3.Event, eventCh chan<- Event) bool {
	for _, ev := range events {
		event := convertEvent(ev)
		select {
		case eventCh <- event:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// convertEvent 将 etcd 事件转换为 xetcd 事件。
func convertEvent(ev *clientv3.Event) Event {
	event := Event{
		Key:      string(ev.Kv.Key),
		Revision: ev.Kv.ModRevision,
	}

	switch ev.Type {
	case mvccpb.PUT:
		event.Type = EventPut
		event.Value = ev.Kv.Value
	case mvccpb.DELETE:
		event.Type = EventDelete
		event.Value = nil
	}

	return event
}
