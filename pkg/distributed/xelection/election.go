package xelection

import "context"

// Election 选举协调者，负责管理一轮或多轮 Leader 竞选。
//
// 并发安全：Election 的所有方法均可被多个 goroutine 并发调用。
// 设计决策：Campaign 与 Close 对 Election 单例共享同一 etcd client，
// 但每次 Campaign 独立创建 Session 与 concurrency.Election，
// 以便 TTL 与 Leader 生命周期按单次当选管理。
type Election interface {
	// Campaign 发起竞选，阻塞直到当选或 ctx 取消。
	// 返回的 Leader 句柄独占一个 etcd Session；其生命周期与调用方 ctx 解耦，
	// 调用方通过 Leader.Resign / Election.Close 主动释放。
	//
	// candidateID 不能为空，通常传入本实例唯一标识（Pod 名、UUID 等）。
	// 当 Election 已被 Close 时返回 ErrElectionClosed。
	Campaign(ctx context.Context, candidateID string) (Leader, error)

	// Close 关闭 Election，释放底层资源。
	// 已由本 Election 产出的 Leader 句柄仍然有效，需各自 Resign 释放。
	// 多次 Close 幂等。
	Close(ctx context.Context) error
}

// Leader 一次当选周期的句柄。
//
// 并发安全：CheckLeader/IsLeader/Lost/CandidateID/Key 可并发调用。
// Resign 多次调用幂等。
type Leader interface {
	// CheckLeader Fencing 前置检查：若仍是 Leader 返回 nil，否则返回 ErrNotLeader。
	// 本方法 O(1)，仅读取本地原子标志，不访问网络。
	CheckLeader() error

	// IsLeader 是否仍是 Leader。语义与 CheckLeader 对等，返回 bool。
	IsLeader() bool

	// Lost 返回一个 channel，当 leadership 丢失时会被关闭。
	// 调用方可用 `<-leader.Lost()` 监听事件，据此重新 Campaign 或触发降级。
	// 多次调用返回同一 channel。
	Lost() <-chan struct{}

	// Resign 主动放弃 leadership 并关闭底层 Session。
	// 多次调用幂等；即使之前已因 Session 过期失去 leadership，Resign 仍可调用以清理资源。
	Resign(ctx context.Context) error

	// CandidateID 本 Leader 的候选者标识。
	CandidateID() string

	// Key 本 Leader 在 etcd 中持有的 key 全路径（prefix + 内部后缀）。
	// 用于外部审计或拒绝环境比对；返回空串表示未当选成功。
	Key() string
}
