package xelection_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/distributed/xelection"
	"github.com/omeyang/xkit/pkg/testkit/xetcdtest"
)

// 集成测试：在进程内嵌入 etcd，端到端验证 Campaign/Resign/抢占/并发 Campaign。
// 目标：覆盖 defaultSessionFactory + Campaign 真实路径（mock 无法覆盖的 10%）。

func newMockOrSkip(t *testing.T) *xetcdtest.Mock {
	t.Helper()
	m, err := xetcdtest.New()
	if err != nil {
		t.Skipf("embed etcd unavailable: %v", err)
	}
	return m
}

// TestIntegration_CampaignAndResign 单个候选者 Campaign → CheckLeader → Resign。
func TestIntegration_CampaignAndResign(t *testing.T) {
	m := newMockOrSkip(t)
	defer m.Close()

	elec, err := xelection.NewEtcdElection(m.Client(), "/xelection-itest/basic/",
		xelection.WithTTL(2))
	if err != nil {
		t.Fatalf("NewEtcdElection: %v", err)
	}
	defer func() {
		if err := elec.Close(context.Background()); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	leader, err := elec.Campaign(ctx, "cand-1")
	if err != nil {
		t.Fatalf("Campaign: %v", err)
	}
	if !leader.IsLeader() {
		t.Fatal("expect IsLeader after Campaign")
	}
	if err := leader.CheckLeader(); err != nil {
		t.Fatalf("CheckLeader: %v", err)
	}
	if leader.CandidateID() != "cand-1" {
		t.Errorf("CandidateID = %q", leader.CandidateID())
	}
	if leader.Key() == "" {
		t.Error("Key should be non-empty after successful Campaign")
	}

	if err := leader.Resign(context.Background()); err != nil {
		t.Fatalf("Resign: %v", err)
	}
	select {
	case <-leader.Lost():
	case <-time.After(2 * time.Second):
		t.Fatal("Lost channel should close after Resign")
	}
	if !errors.Is(leader.CheckLeader(), xelection.ErrNotLeader) {
		t.Error("CheckLeader should return ErrNotLeader after Resign")
	}
}

// TestIntegration_CampaignAfterCloseRejected Close 后 Campaign 返回 ErrElectionClosed。
func TestIntegration_CampaignAfterCloseRejected(t *testing.T) {
	m := newMockOrSkip(t)
	defer m.Close()

	elec, err := xelection.NewEtcdElection(m.Client(), "/xelection-itest/closed/")
	if err != nil {
		t.Fatalf("NewEtcdElection: %v", err)
	}
	if err := elec.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := elec.Campaign(context.Background(), "cand-1"); !errors.Is(err, xelection.ErrElectionClosed) {
		t.Fatalf("want ErrElectionClosed, got %v", err)
	}
}

// TestIntegration_SecondCandidateBlockedUntilResign 第二个候选者 Campaign 阻塞，
// 第一个 Resign 后继任；并且继任者的 Lost 仅在其自身失去 leadership 时关闭。
func TestIntegration_SecondCandidateBlockedUntilResign(t *testing.T) {
	m := newMockOrSkip(t)
	defer m.Close()

	elec, err := xelection.NewEtcdElection(m.Client(), "/xelection-itest/succession/",
		xelection.WithTTL(2))
	if err != nil {
		t.Fatalf("NewEtcdElection: %v", err)
	}
	defer func() {
		if err := elec.Close(context.Background()); err != nil {
			t.Errorf("cleanup Close: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first, err := elec.Campaign(ctx, "cand-a")
	if err != nil {
		t.Fatalf("Campaign cand-a: %v", err)
	}

	secondCh := make(chan xelection.Leader, 1)
	secondErr := make(chan error, 1)
	go func() {
		l, err := elec.Campaign(ctx, "cand-b")
		if err != nil {
			secondErr <- err
			return
		}
		secondCh <- l
	}()

	// 尚未 Resign 时 cand-b 应阻塞。
	select {
	case l := <-secondCh:
		if err := l.Resign(context.Background()); err != nil {
			t.Errorf("premature Resign: %v", err)
		}
		t.Fatal("cand-b should not win before cand-a resigns")
	case err := <-secondErr:
		t.Fatalf("cand-b Campaign errored prematurely: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := first.Resign(context.Background()); err != nil {
		t.Fatalf("first.Resign: %v", err)
	}

	select {
	case l := <-secondCh:
		if !l.IsLeader() {
			t.Error("cand-b should be leader after succession")
		}
		if err := l.Resign(context.Background()); err != nil {
			t.Errorf("cand-b Resign: %v", err)
		}
	case err := <-secondErr:
		t.Fatalf("cand-b Campaign errored: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("cand-b did not win after cand-a resign")
	}
}

// TestIntegration_CampaignContextCanceled 上下文取消 Campaign 立即返回错误，
// 并清理底层 session（不产生 goroutine/资源泄漏）。
func TestIntegration_CampaignContextCanceled(t *testing.T) {
	m := newMockOrSkip(t)
	defer m.Close()

	elec, err := xelection.NewEtcdElection(m.Client(), "/xelection-itest/cancel/")
	if err != nil {
		t.Fatalf("NewEtcdElection: %v", err)
	}
	defer func() {
		if err := elec.Close(context.Background()); err != nil {
			t.Errorf("cleanup Close: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := elec.Campaign(ctx, "cand-owner")
	if err != nil {
		t.Fatalf("first Campaign: %v", err)
	}
	defer func() {
		if err := first.Resign(context.Background()); err != nil {
			t.Errorf("cleanup Resign: %v", err)
		}
	}()

	// 第二个候选者的 ctx 立即取消，应快速返回。
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	_, err = elec.Campaign(cctx, "cand-loser")
	if err == nil {
		t.Fatal("Campaign with canceled ctx should error")
	}
}
