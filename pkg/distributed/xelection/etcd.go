package xelection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xlog"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// 编译期接口实现断言。
var (
	_ Election = (*etcdElection)(nil)
	_ Leader   = (*etcdLeader)(nil)
)

// sessionProvider 抽象 concurrency.Session 的最小集，便于单元测试注入 mock。
// 设计决策：与 xdlock 保持一致的 sessionProvider 命名和职责。
type sessionProvider interface {
	Done() <-chan struct{}
	Close() error
}

// electionKind 抽象 concurrency.Election 供 observe 与 Resign 使用的最小集。
// *concurrency.Election 原生满足此接口（Go 结构类型隐式实现）。
// 设计决策：observe 的 select 分支需要直接消费 clientv3.GetResponse，
// 为避免 mapping 开销与额外 goroutine，接口方法签名与 etcd concurrency 原样一致。
type electionKind interface {
	Observe(ctx context.Context) <-chan clientv3.GetResponse
	Resign(ctx context.Context) error
	Key() string
}

// sessionFactory 负责为每次 Campaign 创建 Session。单元测试可替换为返回 mock。
type sessionFactory func(client *clientv3.Client, ttlSeconds int) (sessionProvider, error)

// defaultSessionFactory 生产默认 Session 工厂，使用 concurrency.NewSession。
func defaultSessionFactory(client *clientv3.Client, ttlSeconds int) (sessionProvider, error) {
	s, err := concurrency.NewSession(client, concurrency.WithTTL(ttlSeconds))
	if err != nil {
		return nil, fmt.Errorf("xelection: create session: %w", err)
	}
	return s, nil
}

// etcdElection 基于 etcd concurrency API 的 Election 实现。
type etcdElection struct {
	client     *clientv3.Client
	prefix     string
	opts       *electionOptions
	sessionFac sessionFactory
	closed     atomic.Bool
}

// NewEtcdElection 创建基于 etcd 的选举协调者。
//
// client 必须为已初始化的 etcd v3 客户端；prefix 为选举 key 前缀（必填，
// 同一 prefix 下所有候选者参与同一选举，且应以 "/" 结尾以避免 prefix 冲突）。
//
// 返回错误：ErrNilClient / ErrEmptyPrefix。
func NewEtcdElection(client *clientv3.Client, prefix string, opts ...Option) (Election, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if prefix == "" {
		return nil, ErrEmptyPrefix
	}
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return &etcdElection{
		client:     client,
		prefix:     prefix,
		opts:       o,
		sessionFac: defaultSessionFactory,
	}, nil
}

// Campaign 参见 Election.Campaign。
func (e *etcdElection) Campaign(ctx context.Context, candidateID string) (Leader, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if candidateID == "" {
		return nil, ErrEmptyCandidateID
	}
	if e.closed.Load() {
		return nil, ErrElectionClosed
	}

	session, err := e.sessionFac(e.client, e.opts.ttlSeconds)
	if err != nil {
		return nil, err
	}
	concSession, ok := session.(*concurrency.Session)
	if !ok {
		// 防御：注入的 sessionProvider 非真实 Session 时，无法创建 Election。
		// 单元测试通过 etcdLeader 直接构造，不走此路径。
		if closeErr := session.Close(); closeErr != nil {
			return nil, fmt.Errorf("xelection: non-etcd session, close: %w", closeErr)
		}
		return nil, fmt.Errorf("xelection: session factory returned non-etcd session")
	}

	elec := concurrency.NewElection(concSession, e.prefix)
	if err := elec.Campaign(ctx, candidateID); err != nil {
		// 同时保留 campaign 与 close 的错误链，便于 errors.Is 判定根因。
		// errors.Join 自动过滤 nil，close 成功时退化为单错误。
		return nil, fmt.Errorf("xelection: campaign: %w",
			errors.Join(err, session.Close()))
	}

	ldr := newEtcdLeader(session, elec, candidateID, e.opts.logger)
	ldr.startObserve()
	return ldr, nil
}

// Close 参见 Election.Close。幂等。
func (e *etcdElection) Close(_ context.Context) error {
	e.closed.Store(true)
	return nil
}

// etcdLeader 一次当选句柄。生命周期：当选 → observe 运行 → 丢失/Resign → 关闭 Session。
type etcdLeader struct {
	session  sessionProvider
	election electionKind
	id       string
	logger   xlog.Logger

	leader  atomic.Bool
	resigns atomic.Bool // 已 Resign 标志，幂等保障

	mu       sync.Mutex
	lostCh   chan struct{}
	lostOnce sync.Once

	// observe 协程生命周期
	observeCtx    context.Context
	observeCancel context.CancelFunc
	observeDone   chan struct{}
}

// newEtcdLeader 构造 Leader 句柄。当选后由 Election 调用。
func newEtcdLeader(session sessionProvider, elec electionKind, id string, logger xlog.Logger) *etcdLeader {
	ctx, cancel := context.WithCancel(context.Background())
	l := &etcdLeader{
		session:       session,
		election:      elec,
		id:            id,
		logger:        logger,
		lostCh:        make(chan struct{}),
		observeCtx:    ctx,
		observeCancel: cancel,
		observeDone:   make(chan struct{}),
	}
	l.leader.Store(true)
	return l
}

// startObserve 启动 leadership 监听 goroutine。
func (l *etcdLeader) startObserve() {
	go l.observe()
}

// observe 监听 election 与 session 事件；任一退出条件触发 lost 并返回。
func (l *etcdLeader) observe() {
	defer close(l.observeDone)
	if l.election == nil { // 单元测试注入路径
		<-l.observeCtx.Done()
		l.loseLeadership("observe ctx done (no election)")
		return
	}
	ch := l.election.Observe(l.observeCtx)
	for {
		select {
		case <-l.observeCtx.Done():
			l.loseLeadership("observe ctx canceled")
			return
		case resp, ok := <-ch:
			if !ok {
				l.loseLeadership("observe channel closed")
				return
			}
			if len(resp.Kvs) > 0 && string(resp.Kvs[0].Value) != l.id {
				l.loseLeadership("preempted by " + string(resp.Kvs[0].Value))
				return
			}
		case <-l.session.Done():
			l.loseLeadership("session expired")
			return
		}
	}
}

// loseLeadership 标记 leadership 丢失并关闭 lostCh。幂等。
func (l *etcdLeader) loseLeadership(reason string) {
	if !l.leader.Swap(false) {
		return
	}
	l.signalLost()
	if l.logger != nil {
		l.logger.Warn(context.Background(), "lost leadership",
			slog.String("id", l.id), slog.String("reason", reason))
	}
}

func (l *etcdLeader) signalLost() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lostOnce.Do(func() { close(l.lostCh) })
}

// CheckLeader 参见 Leader.CheckLeader。
func (l *etcdLeader) CheckLeader() error {
	if !l.leader.Load() {
		return ErrNotLeader
	}
	return nil
}

// IsLeader 参见 Leader.IsLeader。
func (l *etcdLeader) IsLeader() bool { return l.leader.Load() }

// Lost 参见 Leader.Lost。
func (l *etcdLeader) Lost() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lostCh
}

// Resign 参见 Leader.Resign。幂等。
func (l *etcdLeader) Resign(ctx context.Context) error {
	if l.resigns.Swap(true) {
		return nil
	}
	l.leader.Store(false)
	l.signalLost()
	l.observeCancel()
	<-l.observeDone

	var resignErr error
	if l.election != nil {
		if err := l.election.Resign(ctx); err != nil {
			resignErr = fmt.Errorf("xelection: resign: %w", err)
		}
	}
	if err := l.session.Close(); err != nil && resignErr == nil {
		resignErr = fmt.Errorf("xelection: close session: %w", err)
	}
	return resignErr
}

// CandidateID 参见 Leader.CandidateID。
func (l *etcdLeader) CandidateID() string { return l.id }

// Key 参见 Leader.Key。
func (l *etcdLeader) Key() string {
	if l.election == nil {
		return ""
	}
	return l.election.Key()
}
