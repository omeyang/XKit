package xelection

import "errors"

// 参数错误：由调用方传入非法值触发，立即返回，不产生副作用。
var (
	// ErrNilClient 传入的 etcd 客户端为 nil。
	ErrNilClient = errors.New("xelection: nil etcd client")
	// ErrEmptyPrefix 选举 prefix 为空。
	ErrEmptyPrefix = errors.New("xelection: empty prefix")
	// ErrEmptyCandidateID 候选者 ID 为空。
	ErrEmptyCandidateID = errors.New("xelection: empty candidate id")
	// ErrNilContext 传入的 context 为 nil。
	ErrNilContext = errors.New("xelection: nil context")
)

// 运行时错误：在调用某些操作时基于状态返回。
var (
	// ErrNotLeader 当前句柄不再是 Leader（Session 过期 / 被抢占 / 已 Resign）。
	ErrNotLeader = errors.New("xelection: not the leader")
	// ErrElectionClosed Election 已 Close，不能再发起 Campaign。
	ErrElectionClosed = errors.New("xelection: election closed")
)
