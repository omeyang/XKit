package xetcdtest

import (
	"context"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ctx 返回带超时的测试上下文，避免嵌入式 etcd 卡死拖累 CI。
func ctx(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}

func TestNew_PutGetOK(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	c := context.Background()
	if _, err := m.Client().Put(c, "k", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	resp, err := m.Client().Get(c, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Value) != "v" {
		t.Fatalf("unexpected kvs: %+v", resp.Kvs)
	}
}

func TestNew_EndpointsNonEmpty(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	eps := m.Endpoints()
	if len(eps) == 0 {
		t.Fatal("Endpoints should not be empty")
	}
	if !strings.Contains(eps[0], ":") {
		t.Errorf("endpoint %q should contain port", eps[0])
	}
}

func TestNew_WatchReceivesPut(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	c, cancel := ctx(t, 10*time.Second)
	defer cancel()

	ch := m.Client().Watch(c, "/w")
	if _, err := m.Client().Put(c, "/w", "hello"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	select {
	case wr := <-ch:
		if len(wr.Events) == 0 || string(wr.Events[0].Kv.Value) != "hello" {
			t.Fatalf("unexpected watch resp: %+v", wr)
		}
	case <-c.Done():
		t.Fatal("watch timeout")
	}
}

func TestNew_LeaseGrantKeepalive(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	c, cancel := ctx(t, 10*time.Second)
	defer cancel()

	lease, err := m.Client().Grant(c, 5)
	if err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if lease.ID == 0 {
		t.Fatal("lease ID should be non-zero")
	}
	if _, err := m.Client().Put(c, "lk", "lv", clientv3.WithLease(lease.ID)); err != nil {
		t.Fatalf("Put with lease: %v", err)
	}
	if _, err := m.Client().Revoke(c, lease.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
}

func TestNewClient_CleanupCloses(t *testing.T) {
	cli, cleanup, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if cli == nil || cleanup == nil {
		t.Fatal("nil client or cleanup")
	}
	c, cancel := ctx(t, 5*time.Second)
	defer cancel()
	if _, err := cli.Put(c, "x", "y"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	cleanup()
}

func TestClose_IsIdempotent(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Close()
	m.Close() // 二次 Close 不应 panic
}
