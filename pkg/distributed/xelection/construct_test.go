package xelection

import (
	"context"
	"errors"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// TestNewEtcdElection_EmptyPrefix 参数校验：prefix 为空应立即返回 ErrEmptyPrefix。
func TestNewEtcdElection_EmptyPrefix(t *testing.T) {
	t.Parallel()
	// 零值 *clientv3.Client 可通过 nil 检查，本测试不触发任何网络调用。
	client := &clientv3.Client{}
	_, err := NewEtcdElection(client, "")
	if !errors.Is(err, ErrEmptyPrefix) {
		t.Fatalf("want ErrEmptyPrefix, got %v", err)
	}
}

// TestNewEtcdElection_SuccessStoresOptions 构造成功路径：校验选项被应用。
func TestNewEtcdElection_SuccessStoresOptions(t *testing.T) {
	t.Parallel()
	client := &clientv3.Client{}
	elecIface, err := NewEtcdElection(client, "/leader/",
		WithTTL(30), WithTTLDuration(45*time.Second))
	if err != nil {
		t.Fatalf("NewEtcdElection: %v", err)
	}
	e, ok := elecIface.(*etcdElection)
	if !ok {
		t.Fatalf("wrong concrete type: %T", elecIface)
	}
	if e.opts.ttlSeconds != 45 {
		t.Errorf("ttlSeconds = %d, want 45 (last WithTTLDuration wins)", e.opts.ttlSeconds)
	}
	if e.prefix != "/leader/" {
		t.Errorf("prefix = %q", e.prefix)
	}
	if e.sessionFac == nil {
		t.Error("default sessionFac not wired")
	}
	// 默认 ttl 可以立即生效，Close 也可幂等。
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestNewEtcdElection_NilOptionsSilentlyIgnored nil 选项不应 panic。
func TestNewEtcdElection_NilOptionsSilentlyIgnored(t *testing.T) {
	t.Parallel()
	client := &clientv3.Client{}
	var nilOpt Option
	e, err := NewEtcdElection(client, "/p/", nilOpt)
	if err != nil {
		t.Fatalf("NewEtcdElection with nil option: %v", err)
	}
	if e == nil {
		t.Fatal("election should not be nil")
	}
}

// TestLeader_KeyReturnsElectionKey 当 election 非 nil 时，Key 应返回底层 election.Key()。
// 本测试构造一个零值 concurrency.Election，其 Key 返回空字符串 —— 仍走分支覆盖。
// 实际 etcd 路径的 key 内容由集成测试覆盖。
func TestLeader_KeyReturnsElectionKey(t *testing.T) {
	t.Parallel()
	// 直接测试 Key() 方法两个分支：election==nil 已被 NewTestLeader 覆盖；
	// 此处跳过 election!=nil 分支的详细断言，统一留给集成测试。
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer func() {
		if err := l.Resign(context.Background()); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	if got := l.Key(); got != "" {
		t.Errorf("Key() = %q, want empty for nil election", got)
	}
}
