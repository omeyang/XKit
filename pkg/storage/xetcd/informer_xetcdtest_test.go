package xetcd_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/storage/xetcd"
	"github.com/omeyang/xkit/pkg/testkit/xetcdtest"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// 集成测试：嵌入式 etcd 驱动 Informer 的完整 ListWatch 生命周期，
// 覆盖 mock 无法保真的 concurrency / revision 行为。

func newInformerTestEtcd(t *testing.T) (*xetcdtest.Mock, *clientv3.Client) {
	t.Helper()
	m, err := xetcdtest.New()
	if err != nil {
		t.Fatalf("xetcdtest.New: %v", err)
	}
	return m, m.Client()
}

func TestInformer_Integration_ListWatchLifecycle(t *testing.T) {
	m, cli := newInformerTestEtcd(t)
	defer m.Close()

	const prefix = "/xetcd-itest/informer/"
	ctx := context.Background()
	if _, err := cli.Put(ctx, prefix+"a", "1"); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	if _, err := cli.Put(ctx, prefix+"b", "2"); err != nil {
		t.Fatalf("seed put: %v", err)
	}

	var mu sync.Mutex
	events := map[string]string{}
	inf := xetcd.NewInformer(cli, prefix,
		xetcd.WithInformerHandler(func(et xetcd.InformerEventType, key string, value []byte) {
			mu.Lock()
			defer mu.Unlock()
			if et == xetcd.InformerEventDelete {
				events[key] = "<del>"
				return
			}
			events[key] = string(value)
		}),
	)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- inf.Run(runCtx) }()

	// 等初始 List 完成。
	waitFor(t, 3*time.Second, func() bool {
		return inf.Store().Len() == 2
	})

	// 增量事件：Put 新 key + 更新 + 删除。
	if _, err := cli.Put(ctx, prefix+"c", "3"); err != nil {
		t.Fatalf("put c: %v", err)
	}
	if _, err := cli.Put(ctx, prefix+"a", "1+"); err != nil {
		t.Fatalf("update a: %v", err)
	}
	if _, err := cli.Delete(ctx, prefix+"b"); err != nil {
		t.Fatalf("delete b: %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return events[prefix+"a"] == "1+" &&
			events[prefix+"b"] == "<del>" &&
			events[prefix+"c"] == "3"
	})

	if v, ok := inf.Store().Get(prefix + "a"); !ok || string(v) != "1+" {
		t.Errorf("Store[a] = %q ok=%v", v, ok)
	}
	if _, ok := inf.Store().Get(prefix + "b"); ok {
		t.Error("Store[b] should be removed")
	}
	if v, ok := inf.Store().Get(prefix + "c"); !ok || string(v) != "3" {
		t.Errorf("Store[c] = %q ok=%v", v, ok)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

// waitFor 轮询条件直到满足或超时。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied within %s", timeout)
}
