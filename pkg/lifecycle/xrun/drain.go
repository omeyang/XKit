package xrun

import (
	"context"
	"sync"
)

type drainState struct {
	inflight int
	closed   bool
	done     chan struct{}
}

// Drainer 按资源 key 跟踪在途操作，并协调一次排水边界。
// 排水开始后，该资源不再接受新操作；历史空闲不会重置此边界。
type Drainer struct {
	mu        sync.Mutex
	resources map[string]*drainState
}

// NewDrainer 创建按资源隔离的排水器。
func NewDrainer() *Drainer {
	return &Drainer{resources: map[string]*drainState{}}
}

func (d *Drainer) state(resource string) *drainState {
	state, ok := d.resources[resource]
	if !ok {
		state = &drainState{done: make(chan struct{})}
		d.resources[resource] = state
	}
	return state
}

// Begin 登记一个在途操作；资源排水开始后返回 ErrDraining。
func (d *Drainer) Begin(resource string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.state(resource)
	if state.closed {
		return ErrDraining
	}
	state.inflight++
	return nil
}

// End 标记一个已登记操作结束，并在排水期间归零时唤醒等待者。
func (d *Drainer) End(resource string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.state(resource)
	if state.inflight > 0 {
		state.inflight--
	}
	if state.closed && state.inflight == 0 {
		select {
		case <-state.done:
		default:
			close(state.done)
		}
	}
}

// Drain 停止资源接纳新操作，并等待当前在途操作结束或 ctx 取消。
func (d *Drainer) Drain(ctx context.Context, resource string) error {
	d.mu.Lock()
	state := d.state(resource)
	state.closed = true
	if state.inflight == 0 {
		d.mu.Unlock()
		return nil
	}
	done := state.done
	d.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Accepting 报告资源是否仍接受新操作；未知资源默认接受。
func (d *Drainer) Accepting(resource string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, exists := d.resources[resource]
	return !exists || !state.closed
}
