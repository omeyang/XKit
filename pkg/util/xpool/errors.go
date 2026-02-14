package xpool

import "errors"

var (
	// ErrNilHandler 表示 handler 参数为 nil。
	ErrNilHandler = errors.New("xpool: handler cannot be nil")

	// ErrPoolStopped 表示 worker pool 已停止，无法提交任务。
	ErrPoolStopped = errors.New("xpool: pool is stopped")

	// ErrQueueFull 表示任务队列已满，任务被丢弃。
	ErrQueueFull = errors.New("xpool: queue is full, task dropped")
)
