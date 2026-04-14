package xelection

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// 本文件覆盖 2026-04-14 对抗审查补修的两项行为契约：
//   - FG-H: observe 在 watch 中断 / session 过期 / 被抢占时主动释放 lease
//   - FG-M: Election.Close 能打断已阻塞的 in-flight Campaign 并幂等

// TestObserve_ChannelClosedReleasesSession 模拟 watch 中断。
// 修复前：loseLeadership 只关 lostCh，旧 session 仍 keepalive 占着 leader key。
// 修复后：observe 必须调用 releaseSession 释放 lease。
func TestObserve_ChannelClosedReleasesSession(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	me.CloseEvents() // 模拟 watch 通道中断

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.WaitObserveDone(ctx); err != nil {
		t.Fatalf("observe did not exit: %v", err)
	}
	if !ms.Closed() {
		t.Fatal("session must be closed after observe channel closed (FG-H fix)")
	}
}

// TestObserve_SessionExpiredReleasesSession 覆盖 session 过期分支。
// releaseSession 虽与已过期 session 的 Close 幂等，也要被调用以保持语义对称。
func TestObserve_SessionExpiredReleasesSession(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	ms.Expire()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.WaitObserveDone(ctx); err != nil {
		t.Fatalf("observe did not exit: %v", err)
	}
	if !ms.Closed() {
		t.Fatal("session must be marked closed after expiry")
	}
}

// TestObserve_PreemptedReleasesSession 被抢占时也应释放自己的 session。
func TestObserve_PreemptedReleasesSession(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	me.SendLeader("other")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.WaitObserveDone(ctx); err != nil {
		t.Fatalf("observe did not exit: %v", err)
	}
	if !ms.Closed() {
		t.Fatal("session must be closed after preemption")
	}
}

// TestResign_SessionClosedOnceEvenAfterObserveRelease 并发场景：observe 因 watch 中断
// 先 releaseSession，之后用户仍调用 Resign；session.Close 只应被执行一次（sync.Once 幂等）。
func TestResign_SessionClosedOnceEvenAfterObserveRelease(t *testing.T) {
	t.Parallel()
	ms := &countingSession{doneCh: make(chan struct{})}
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	me.CloseEvents()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.WaitObserveDone(ctx); err != nil {
		t.Fatal(err)
	}

	if err := l.Resign(context.Background()); err != nil {
		t.Errorf("Resign after observe release: %v", err)
	}
	if got := ms.closeCount.Load(); got != 1 {
		t.Fatalf("session.Close calls = %d, want 1 (sync.Once guard)", got)
	}
}

// TestResign_CollectsSessionCloseError observe 释放 session 失败后，
// Resign 应通过 releaseSession 观察到同一个错误（sync.Once 捕获首次错误）。
func TestResign_CollectsSessionCloseError(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	closeErr := errors.New("mock close failure")
	ms.SetCloseErr(closeErr)
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	me.CloseEvents()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.WaitObserveDone(ctx); err != nil {
		t.Fatal(err)
	}

	// Resign 此时应能取回 observe 捕获的那个错误。
	err := l.Resign(context.Background())
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("Resign err = %v, want wrap %v", err, closeErr)
	}
}

// TestElection_CloseIdempotentCancelsCloseCtx Close 多次调用只触发一次 cancel；
// closeCtx 必须在首次 Close 后被取消（下游 Campaign watcher goroutine 监听它）。
func TestElection_CloseIdempotentCancelsCloseCtx(t *testing.T) {
	t.Parallel()
	e := NewTestElection("test/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})

	if err := e.Close(context.Background()); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	select {
	case <-e.closeCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("closeCtx must be canceled after Close (FG-M fix)")
	}
	// 再次 Close 应幂等，不 panic（double-cancel 安全 but Swap 守护）
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestElection_CampaignAfterCloseReturnsErrClosed 关闭后 Campaign 立即返回 ErrElectionClosed。
// 即使 in-flight watcher 方案没触发（入口早检查），契约也应一致。
func TestElection_CampaignAfterCloseReturnsErrClosed(t *testing.T) {
	t.Parallel()
	e := NewTestElection("test/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})
	if err := e.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := e.Campaign(context.Background(), "cand-1")
	if !errors.Is(err, ErrElectionClosed) {
		t.Fatalf("Campaign after Close err = %v, want ErrElectionClosed", err)
	}
}

// countingSession 用于验证 Close 幂等次数。
type countingSession struct {
	doneCh     chan struct{}
	closeCount atomic.Int64
}

func (c *countingSession) Done() <-chan struct{} { return c.doneCh }
func (c *countingSession) Close() error {
	c.closeCount.Add(1)
	return nil
}
