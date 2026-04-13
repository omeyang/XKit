package xelection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// discardLoggerForObserveTest 返回一个非 nil 的默认 Logger，
// 用于覆盖 loseLeadership 的 logger != nil 分支。
func discardLoggerForObserveTest() xlog.Logger { return xlog.Default() }

// resignQuietly 在 defer 中清理 Leader，errcheck 约束下将错误显式检查。
func resignQuietly(l *etcdLeader) {
	if err := l.Resign(context.Background()); err != nil {
		_ = err
	}
}

// observe 路径的单元测试：通过 MockElection/MockSession 驱动所有分支，
// 无需真实 etcd。目标：将 observe() 覆盖率从 27.8% 提升到 100%。

const observeWaitTimeout = 2 * time.Second

// TestObserve_PreemptedTriggersLoss 对手 candidate 写入不同的 leader value，
// observe 应立即判定 preempted 并关闭 Lost。
func TestObserve_PreemptedTriggersLoss(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	lost := l.Lost()
	me.SendLeader("other-node") // 推送被抢占事件

	select {
	case <-lost:
	case <-time.After(observeWaitTimeout):
		t.Fatal("Lost not signaled after preemption")
	}
	if l.IsLeader() {
		t.Error("IsLeader should be false after preemption")
	}
	if err := l.Resign(context.Background()); err != nil {
		t.Errorf("cleanup Resign: %v", err)
	}
}

// TestObserve_SameLeaderIgnored 对 leader value 与自身一致的事件不应触发 loss。
func TestObserve_SameLeaderIgnored(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)
	defer resignQuietly(l)

	me.SendLeader("self") // 同 id，observe 应继续循环
	me.SendLeader("self") // 再来一次

	// 留短时间让 observe 处理事件，然后检查仍是 leader
	time.Sleep(50 * time.Millisecond)
	if !l.IsLeader() {
		t.Error("same-id event should not drop leadership")
	}
}

// TestObserve_EmptyKvsIgnored Kvs 为空时（例如 leader 刚被删除但 observe 尚未收到
// 新 leader）应继续等待，不误判 loss。
func TestObserve_EmptyKvsIgnored(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)
	defer resignQuietly(l)

	me.SendLeader("") // 空 Kvs

	time.Sleep(50 * time.Millisecond)
	if !l.IsLeader() {
		t.Error("empty-kvs event should not drop leadership")
	}
}

// TestObserve_ChannelClosedTriggersLoss 底层 observe channel 关闭（etcd 断连等）
// 应立即判定 loss 并关闭 Lost。
func TestObserve_ChannelClosedTriggersLoss(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	lost := l.Lost()
	me.CloseEvents()

	select {
	case <-lost:
	case <-time.After(observeWaitTimeout):
		t.Fatal("Lost not signaled after channel close")
	}
	if l.IsLeader() {
		t.Error("IsLeader should be false after channel close")
	}
	if err := l.Resign(context.Background()); err != nil {
		t.Errorf("cleanup Resign: %v", err)
	}
}

// TestObserve_SessionExpiredTriggersLoss Session Done channel 关闭应判定 loss。
func TestObserve_SessionExpiredTriggersLoss(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	lost := l.Lost()
	ms.Expire()

	select {
	case <-lost:
	case <-time.After(observeWaitTimeout):
		t.Fatal("Lost not signaled after session expiry")
	}
	if l.IsLeader() {
		t.Error("IsLeader should be false after session expiry")
	}
	if err := l.Resign(context.Background()); err != nil {
		t.Errorf("cleanup Resign: %v", err)
	}
}

// TestObserve_ResignCancelsObserveCtx Resign 后 observe goroutine 应退出。
func TestObserve_ResignCancelsObserveCtx(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	if err := l.Resign(context.Background()); err != nil {
		t.Fatalf("Resign: %v", err)
	}
	// Resign 会等待 observeDone；能走到这里说明 goroutine 已退出。
	if !me.Resigned {
		t.Error("election.Resign should be called during Leader.Resign")
	}
}

// TestResign_ElectionResignErrorPropagates Election.Resign 错误应被包装传播。
func TestResign_ElectionResignErrorPropagates(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	sentinel := errors.New("resign boom")
	me.ResignErr = sentinel
	l := NewTestLeaderWithElection(ms, me, "self", nil)

	err := l.Resign(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("want wrapped sentinel, got %v", err)
	}
}

// TestLeader_KeyFromElection 注入的 election 应被用于 Key() 返回值。
func TestLeader_KeyFromElection(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	me.KeyVal = "/myapp/leader/abc"
	l := NewTestLeaderWithElection(ms, me, "self", nil)
	defer resignQuietly(l)

	if got := l.Key(); got != "/myapp/leader/abc" {
		t.Errorf("Key() = %q", got)
	}
}

// TestLoseLeadership_WithLogger 带 logger 的 loss 路径；覆盖 loseLeadership 的
// logger != nil 分支（之前单元测试 logger 均为 nil）。
func TestLoseLeadership_WithLogger(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	me := NewMockElection()
	// 使用 nil 之外的任意 Logger 实例；此处 Warn 本身只要不 panic 即达目的。
	l := NewTestLeaderWithElection(ms, me, "self", discardLoggerForObserveTest())
	defer resignQuietly(l)

	lost := l.Lost()
	l.TriggerLose("with-logger")
	select {
	case <-lost:
	case <-time.After(observeWaitTimeout):
		t.Fatal("Lost not signaled")
	}
}
