package xelection

import (
	"context"

	"github.com/omeyang/xkit/pkg/observability/xlog"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// 测试辅助：暴露内部构造器供单元测试直接操纵 etcdLeader 与 etcdElection，
// 避免依赖真实 etcd 即可覆盖分支逻辑。

// MockSession 实现 sessionProvider，允许测试控制 Done 与 Close。
type MockSession struct {
	doneCh   chan struct{}
	closeErr error
	closed   bool
}

// NewMockSession 返回未过期、Close 无错误的 MockSession。
func NewMockSession() *MockSession {
	return &MockSession{doneCh: make(chan struct{})}
}

// NewExpiredMockSession 返回已过期的 MockSession（Done channel 已关闭）。
func NewExpiredMockSession() *MockSession {
	ch := make(chan struct{})
	close(ch)
	return &MockSession{doneCh: ch}
}

// Done 参见 sessionProvider。
func (m *MockSession) Done() <-chan struct{} { return m.doneCh }

// Close 参见 sessionProvider。
func (m *MockSession) Close() error {
	m.closed = true
	return m.closeErr
}

// Closed 报告 Close 是否被调用。
func (m *MockSession) Closed() bool { return m.closed }

// SetCloseErr 设置 Close 返回的错误。
func (m *MockSession) SetCloseErr(err error) { m.closeErr = err }

// Expire 手动触发 Session 过期（关闭 doneCh）。
func (m *MockSession) Expire() { close(m.doneCh) }

// NewTestLeader 构造一个无真实 etcd Election 的 Leader，用于单元测试
// 覆盖 CheckLeader/IsLeader/Lost/Resign 分支。observe goroutine 会阻塞
// 在 observeCtx 上，直到 Resign 触发 cancel。
func NewTestLeader(session *MockSession, id string, logger xlog.Logger) *etcdLeader {
	l := newEtcdLeader(session, nil, id, logger)
	l.startObserve()
	return l
}

// MockElection 实现 electionKind，用于驱动 observe 分支的单元测试。
// Events 写入 Events channel 即可被 observe 消费；RespWithLeader 生成带
// 指定 leader 值的 GetResponse，便于构造被抢占或未抢占场景。
type MockElection struct {
	Events    chan clientv3.GetResponse
	ResignErr error
	Resigned  bool
	KeyVal    string
}

// NewMockElection 创建带缓冲的 MockElection，容量足够单测场景。
func NewMockElection() *MockElection {
	return &MockElection{Events: make(chan clientv3.GetResponse, 4)}
}

// Observe 参见 electionKind。返回 MockElection.Events 的只读视图。
func (m *MockElection) Observe(_ context.Context) <-chan clientv3.GetResponse { return m.Events }

// Resign 参见 electionKind。返回预设 Err 并标记 Resigned。
func (m *MockElection) Resign(_ context.Context) error { m.Resigned = true; return m.ResignErr }

// Key 参见 electionKind。返回预设字符串。
func (m *MockElection) Key() string { return m.KeyVal }

// CloseEvents 关闭 Events channel（触发 observe 的 "channel closed" 分支）。
func (m *MockElection) CloseEvents() { close(m.Events) }

// SendLeader 推送一条表示指定 leader 的 observe 事件。value 为空时 Kvs 为空
// 切片，用于构造 "当前无 leader" 分支。
func (m *MockElection) SendLeader(value string) {
	if value == "" {
		m.Events <- clientv3.GetResponse{}
		return
	}
	m.Events <- clientv3.GetResponse{
		Kvs: []*mvccpb.KeyValue{{Value: []byte(value)}},
	}
}

// NewTestLeaderWithElection 构造一个注入 MockElection 的 Leader，
// 用于完整覆盖 observe 的所有分支（抢占/通道关闭/session 过期）。
// session 接受 sessionProvider 以便测试注入自定义实现（如计数型 session）。
func NewTestLeaderWithElection(session sessionProvider, elec electionKind, id string, logger xlog.Logger) *etcdLeader {
	l := newEtcdLeader(session, elec, id, logger)
	l.startObserve()
	return l
}

// NewTestElection 构造一个可注入 sessionFactory 的 Election，用于单元测试
// Campaign 路径（factory 返回的 session 非 *concurrency.Session 时走错误分支）。
func NewTestElection(prefix string, fac func() (sessionProvider, error), opts ...Option) *etcdElection {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	closeCtx, closeCancel := context.WithCancel(context.Background())
	return &etcdElection{
		prefix: prefix,
		opts:   o,
		sessionFac: func(_ *clientv3.Client, _ int) (sessionProvider, error) {
			return fac()
		},
		closeCtx:    closeCtx,
		closeCancel: closeCancel,
	}
}

// SetClosed 直接将 Election 标记为已关闭（测试 ErrElectionClosed 路径）。
func (e *etcdElection) SetClosed() { e.closed.Store(true) }

// TriggerLose 直接触发 leadership 丢失（测试 Lost channel 路径）。
func (l *etcdLeader) TriggerLose(reason string) { l.loseLeadership(reason) }

// WaitObserveDone 等待 observe goroutine 退出（测试辅助，避免 flaky）。
func (l *etcdLeader) WaitObserveDone(ctx context.Context) error {
	select {
	case <-l.observeDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
