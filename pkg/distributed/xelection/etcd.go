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

	// closeCtx 在 Close 时被 cancel，用于打断在 elec.Campaign(ctx, ...) 里
	// 已经阻塞的 in-flight 候选者：单独的 watcher goroutine 监听 closeCtx，
	// 一旦触发就 cancel 派生出的 campaignCtx，让 etcd concurrency 返回错误。
	closeCtx    context.Context
	closeCancel context.CancelFunc
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
	closeCtx, closeCancel := context.WithCancel(context.Background())
	return &etcdElection{
		client:      client,
		prefix:      prefix,
		opts:        o,
		sessionFac:  defaultSessionFactory,
		closeCtx:    closeCtx,
		closeCancel: closeCancel,
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

	session, concSession, err := e.acquireSession()
	if err != nil {
		return nil, err
	}

	elec := concurrency.NewElection(concSession, e.prefix)
	if err := e.runCampaign(ctx, elec, candidateID); err != nil {
		return nil, e.handleCampaignErr(err, session)
	}

	// 防御竞态：Campaign 成功返回期间 Close 可能已触发；此时该句柄不应被使用，
	// 立即 Resign 并关闭 session 以释放 leader key，返回 ErrElectionClosed。
	if e.closed.Load() {
		resignErr := elec.Resign(context.Background())
		return nil, errors.Join(ErrElectionClosed, resignErr, session.Close())
	}

	ldr := newEtcdLeader(session, elec, candidateID, e.opts.logger)
	ldr.startObserve()
	return ldr, nil
}

// acquireSession 通过注入工厂创建 session 并校验其为真实 *concurrency.Session。
// 非真实 Session 时会尝试 Close 并返回错误，避免 lease 泄漏。
func (e *etcdElection) acquireSession() (sessionProvider, *concurrency.Session, error) {
	session, err := e.sessionFac(e.client, e.opts.ttlSeconds)
	if err != nil {
		return nil, nil, err
	}
	concSession, ok := session.(*concurrency.Session)
	if !ok {
		if closeErr := session.Close(); closeErr != nil {
			return nil, nil, fmt.Errorf("xelection: non-etcd session, close: %w", closeErr)
		}
		return nil, nil, fmt.Errorf("xelection: session factory returned non-etcd session")
	}
	return session, concSession, nil
}

// runCampaign 运行 elec.Campaign；派生 campaignCtx 使 e.Close 能打断已阻塞的候选者
// （修复 FG-M：原实现 Close 只置标志，无法中断底层 elec.Campaign）。
func (e *etcdElection) runCampaign(ctx context.Context, elec *concurrency.Election, candidateID string) error {
	campaignCtx, campaignCancel := context.WithCancel(ctx)
	defer campaignCancel()
	stopWatch := make(chan struct{})
	go func() {
		select {
		case <-e.closeCtx.Done():
			campaignCancel()
		case <-stopWatch:
		}
	}()
	defer close(stopWatch)
	return elec.Campaign(campaignCtx, candidateID)
}

// handleCampaignErr 统一 Campaign 错误处理：Close 触发的打断归一为 ErrElectionClosed，
// 其他错误保留原 cause 链；两种路径都回收 session 避免 lease 泄漏。
func (e *etcdElection) handleCampaignErr(err error, session sessionProvider) error {
	if e.closed.Load() {
		return errors.Join(ErrElectionClosed, session.Close())
	}
	return fmt.Errorf("xelection: campaign: %w",
		errors.Join(err, session.Close()))
}

// Close 参见 Election.Close。幂等；同时 cancel closeCtx，打断已阻塞的 in-flight Campaign。
func (e *etcdElection) Close(_ context.Context) error {
	if e.closed.Swap(true) {
		return nil
	}
	e.closeCancel()
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

	// session 释放幂等守护：observe 在 watch 中断/session 过期时主动释放 lease，
	// Resign 走 election.Resign → releaseSession 的顺序；任一路径先执行后，
	// 另一路径调用 releaseSession 即为 no-op（sync.Once 保证）。
	closeOnce       sync.Once
	sessionCloseErr error
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
//
// 修复 FG-H：watch 中断（observe channel closed）或 session 过期时，
// 必须主动释放 lease（关闭 session），否则调用方按 Lost() 重新 Campaign
// 会被同进程旧 leader key 长时间阻塞直到 TTL 到期。
// ctx 被主动取消的分支（由 Resign 触发）不在此处释放，让 Resign 在
// election.Resign 成功后再 releaseSession，保持 revoke 顺序正确。
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
				l.releaseSessionLogged("observe channel closed") // 释放 lease，避免旧 key 阻塞后续 Campaign
				return
			}
			if len(resp.Kvs) > 0 && string(resp.Kvs[0].Value) != l.id {
				l.loseLeadership("preempted by " + string(resp.Kvs[0].Value))
				l.releaseSessionLogged("preempted")
				return
			}
		case <-l.session.Done():
			l.loseLeadership("session expired")
			l.releaseSessionLogged("session expired")
			return
		}
	}
}

// releaseSession 幂等关闭 session，捕获并记录首次 Close 错误。
// observe 与 Resign 共用此方法确保 session 只关闭一次。
func (l *etcdLeader) releaseSession() error {
	l.closeOnce.Do(func() {
		if err := l.session.Close(); err != nil {
			l.sessionCloseErr = fmt.Errorf("xelection: close session: %w", err)
		}
	})
	return l.sessionCloseErr
}

// releaseSessionLogged 在 observe 退出路径调用 releaseSession，错误经 logger 记录
// 后吞掉（observe goroutine 无调用者可接收返回值）。Resign 路径走 releaseSession
// 直接返回错误给调用者。
func (l *etcdLeader) releaseSessionLogged(reason string) {
	if err := l.releaseSession(); err != nil && l.logger != nil {
		l.logger.Warn(context.Background(), "xelection: release session failed",
			slog.String("id", l.id),
			slog.String("reason", reason),
			slog.String("err", err.Error()))
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

	// 同时保留 election.Resign 与 session.Close 错误链：任一失败都要可被 errors.Is 识别。
	// 用 errors.Join 避免 resignErr != nil 时静默丢弃 session.Close 错误。
	// releaseSession 与 observe 共享 sync.Once，避免双关闭。
	var electionErr error
	if l.election != nil {
		if err := l.election.Resign(ctx); err != nil {
			electionErr = fmt.Errorf("xelection: resign: %w", err)
		}
	}
	return errors.Join(electionErr, l.releaseSession())
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
