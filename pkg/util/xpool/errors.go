package xpool

import "errors"

var (
	// ErrNilHandler 表示 handler 参数为 nil。
	ErrNilHandler = errors.New("xpool: handler cannot be nil")

	// ErrPoolStopped 表示 worker pool 已关闭，无法提交任务。
	ErrPoolStopped = errors.New("xpool: pool is stopped")

	// ErrQueueFull 表示任务队列已满。
	ErrQueueFull = errors.New("xpool: queue is full")

	// ErrInvalidWorkers 表示 worker 数量无效。
	ErrInvalidWorkers = errors.New("xpool: invalid worker count")

	// ErrInvalidQueueSize 表示队列大小无效。
	ErrInvalidQueueSize = errors.New("xpool: invalid queue size")

	// ErrNilContext 表示 context 参数为 nil。
	ErrNilContext = errors.New("xpool: nil context")
)
