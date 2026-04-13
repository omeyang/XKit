package xelection

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 约定：单元测试不依赖真实 etcd。通过 New{Test,Mock}* 构造器
// 注入 mock，覆盖所有可单测分支（状态机、幂等、并发、错误传播）。

// --- NewEtcdElection 参数校验 ---

func TestNewEtcdElection_NilClient(t *testing.T) {
	t.Parallel()
	_, err := NewEtcdElection(nil, "/p/")
	if !errors.Is(err, ErrNilClient) {
		t.Fatalf("want ErrNilClient, got %v", err)
	}
}

// --- Campaign 参数校验（通过注入 sessionFactory 不依赖真实 etcd）---

func TestCampaign_NilContext(t *testing.T) {
	t.Parallel()
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})
	_, err := e.Campaign(nil, "cand-1") //nolint:staticcheck // SA1012: nil ctx 是测试目标
	if !errors.Is(err, ErrNilContext) {
		t.Fatalf("want ErrNilContext, got %v", err)
	}
}

func TestCampaign_EmptyCandidateID(t *testing.T) {
	t.Parallel()
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})
	_, err := e.Campaign(context.Background(), "")
	if !errors.Is(err, ErrEmptyCandidateID) {
		t.Fatalf("want ErrEmptyCandidateID, got %v", err)
	}
}

func TestCampaign_ElectionClosed(t *testing.T) {
	t.Parallel()
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})
	e.SetClosed()
	_, err := e.Campaign(context.Background(), "cand-1")
	if !errors.Is(err, ErrElectionClosed) {
		t.Fatalf("want ErrElectionClosed, got %v", err)
	}
}

func TestCampaign_SessionFactoryError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("dial failed")
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return nil, sentinel
	})
	_, err := e.Campaign(context.Background(), "cand-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("want wrapped sentinel, got %v", err)
	}
}

func TestCampaign_NonEtcdSessionRejected(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return ms, nil
	})
	_, err := e.Campaign(context.Background(), "cand-1")
	if err == nil {
		t.Fatal("want error for non-etcd session")
	}
	if !strings.Contains(err.Error(), "non-etcd session") {
		t.Fatalf("want 'non-etcd session' in err, got %v", err)
	}
	if !ms.Closed() {
		t.Fatal("session should be closed after defensive rejection")
	}
}

func TestCampaign_NonEtcdSessionCloseFails(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	ms.SetCloseErr(errors.New("close boom"))
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return ms, nil
	})
	_, err := e.Campaign(context.Background(), "cand-1")
	if err == nil || !strings.Contains(err.Error(), "close") {
		t.Fatalf("want close err, got %v", err)
	}
}

func TestElection_CloseIsIdempotent(t *testing.T) {
	t.Parallel()
	e := NewTestElection("/p/", func() (sessionProvider, error) {
		return NewMockSession(), nil
	})
	if err := e.Close(context.Background()); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := e.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// --- Leader 单元测试（无真实 etcd）---

func TestLeader_InitialStateIsLeader(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer resignNoErr(t, l)

	if !l.IsLeader() {
		t.Fatal("want IsLeader true after construction")
	}
	if err := l.CheckLeader(); err != nil {
		t.Fatalf("CheckLeader: %v", err)
	}
	if l.CandidateID() != "cand-1" {
		t.Errorf("CandidateID = %q", l.CandidateID())
	}
	if l.Key() != "" {
		t.Errorf("Key should be empty for test leader (no election), got %q", l.Key())
	}
}

func TestLeader_TriggerLoseClosesLostCh(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer resignNoErr(t, l)

	lost := l.Lost()
	l.TriggerLose("test")

	select {
	case <-lost:
	case <-time.After(2 * time.Second):
		t.Fatal("Lost() channel not closed")
	}
	if l.IsLeader() {
		t.Error("IsLeader should be false after loss")
	}
	if !errors.Is(l.CheckLeader(), ErrNotLeader) {
		t.Error("CheckLeader should return ErrNotLeader after loss")
	}
}

func TestLeader_TriggerLoseIsIdempotent(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer resignNoErr(t, l)

	lost := l.Lost()
	l.TriggerLose("first")
	l.TriggerLose("second") // close 已关闭 channel 会 panic，幂等保证不再 close

	select {
	case <-lost:
	default:
		t.Fatal("Lost should already be closed")
	}
}

func TestLeader_LostChannelStable(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer resignNoErr(t, l)

	a := l.Lost()
	b := l.Lost()
	if a != b {
		t.Fatal("Lost() should return the same channel on repeated calls")
	}
}

func TestLeader_ResignClosesSession(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)

	if err := l.Resign(context.Background()); err != nil {
		t.Fatalf("Resign: %v", err)
	}
	if !ms.Closed() {
		t.Error("session should be closed after Resign")
	}
	if l.IsLeader() {
		t.Error("IsLeader should be false after Resign")
	}
	if !errors.Is(l.CheckLeader(), ErrNotLeader) {
		t.Error("CheckLeader should return ErrNotLeader after Resign")
	}
}

func TestLeader_ResignIdempotent(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)

	if err := l.Resign(context.Background()); err != nil {
		t.Fatalf("first Resign: %v", err)
	}
	ms.SetCloseErr(errors.New("should not be called"))
	if err := l.Resign(context.Background()); err != nil {
		t.Fatalf("second Resign: %v", err)
	}
}

func TestLeader_ResignPropagatesSessionCloseError(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	sentinel := errors.New("close boom")
	ms.SetCloseErr(sentinel)
	l := NewTestLeader(ms, "cand-1", nil)
	err := l.Resign(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("want wrapped sentinel, got %v", err)
	}
}

func TestLeader_ConcurrentCheckLeader(t *testing.T) {
	t.Parallel()
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer resignNoErr(t, l)

	const goroutines = 16
	const iters = 2000
	var wg sync.WaitGroup
	var calls atomic.Int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				if err := l.CheckLeader(); err != nil {
					return
				}
				calls.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != int64(goroutines*iters) {
		t.Errorf("call count mismatch: %d", got)
	}
}

// resignNoErr 确保 observe goroutine 清理，避免 goleak。
func resignNoErr(t *testing.T, l *etcdLeader) {
	t.Helper()
	if err := l.Resign(context.Background()); err != nil {
		t.Errorf("cleanup Resign: %v", err)
	}
}
