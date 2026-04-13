package xetcd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// mockListWatcher 实现 informerListWatcher，用于无真实 etcd 的 Informer 单测。
// 设计决策：Watch() 每次调用返回全新 channel；sendPut/sendDelete/closeWatch 操作
// "当前" watch channel。closeWatch 后下一次 Watch() 产生新的活动 channel。
type mockListWatcher struct {
	mu       sync.Mutex
	getResps []mockGetResp
	watchCh  chan clientv3.WatchResponse // 当前活动 channel，nil 表示尚未 Watch
	getCalls atomic.Int32
	wchCalls atomic.Int32
}

type mockGetResp struct {
	kvs []*mvccpb.KeyValue
	rev int64
	err error
}

func newMockListWatcher() *mockListWatcher { return &mockListWatcher{} }

func (m *mockListWatcher) queueGet(r mockGetResp) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getResps = append(m.getResps, r)
}

func (m *mockListWatcher) Get(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	m.getCalls.Add(1)
	m.mu.Lock()
	if len(m.getResps) == 0 {
		m.mu.Unlock()
		return &clientv3.GetResponse{Header: &etcdserverpb.ResponseHeader{Revision: 1}}, nil
	}
	r := m.getResps[0]
	m.getResps = m.getResps[1:]
	m.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: r.rev},
		Kvs:    r.kvs,
	}, nil
}

func (m *mockListWatcher) Watch(_ context.Context, _ string, _ ...clientv3.OpOption) clientv3.WatchChan {
	m.wchCalls.Add(1)
	ch := make(chan clientv3.WatchResponse, 16)
	m.mu.Lock()
	m.watchCh = ch
	m.mu.Unlock()
	return ch
}

func (m *mockListWatcher) currentWatchCh() chan clientv3.WatchResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.watchCh
}

func (m *mockListWatcher) sendPut(key, value string, rev int64) {
	ch := m.currentWatchCh()
	if ch == nil {
		return
	}
	ch <- clientv3.WatchResponse{
		Events: []*clientv3.Event{{
			Type: clientv3.EventTypePut,
			Kv:   &mvccpb.KeyValue{Key: []byte(key), Value: []byte(value), ModRevision: rev},
		}},
	}
}

func (m *mockListWatcher) sendDelete(key string, rev int64) {
	ch := m.currentWatchCh()
	if ch == nil {
		return
	}
	ch <- clientv3.WatchResponse{
		Events: []*clientv3.Event{{
			Type: clientv3.EventTypeDelete,
			Kv:   &mvccpb.KeyValue{Key: []byte(key), ModRevision: rev},
		}},
	}
}

func (m *mockListWatcher) sendProgressNotify() {
	ch := m.currentWatchCh()
	if ch == nil {
		return
	}
	// IsProgressNotify 返回 true 的条件：Events 空且 Canceled=false。
	ch <- clientv3.WatchResponse{}
}

// closeWatch 关闭当前活动 channel 并清除引用，促使下一次 Watch() 获得新 channel。
func (m *mockListWatcher) closeWatch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.watchCh != nil {
		close(m.watchCh)
		m.watchCh = nil
	}
}

// runInBackground 返回 Run 的错误 channel，便于 cancel 后读取。
func runInBackground(t *testing.T, inf *Informer, ctx context.Context) <-chan error {
	t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- inf.Run(ctx) }()
	return ch
}

func waitForLen(t *testing.T, s *InformerStore, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.Len() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout: store len = %d, want >= %d", s.Len(), n)
}

// TestInformer_InitialListPopulatesStore 初始 List 应将所有 kv 放入 Store。
func TestInformer_InitialListPopulatesStore(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{
		rev: 5,
		kvs: []*mvccpb.KeyValue{
			{Key: []byte("/p/a"), Value: []byte("va"), ModRevision: 1},
			{Key: []byte("/p/b"), Value: []byte("vb"), ModRevision: 2},
		},
	})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForLen(t, inf.Store(), 2)
	if inf.Store().Rev() != 5 {
		t.Errorf("rev = %d, want 5", inf.Store().Rev())
	}
	cancel()
	<-errCh
}

// TestInformer_HandlerCalledForInitialKeys 初始 List 每个 key 触发一次 Put 事件。
func TestInformer_HandlerCalledForInitialKeys(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{
		rev: 3,
		kvs: []*mvccpb.KeyValue{
			{Key: []byte("/p/x"), Value: []byte("vx"), ModRevision: 1},
			{Key: []byte("/p/y"), Value: []byte("vy"), ModRevision: 2},
		},
	})
	var mu sync.Mutex
	seen := map[string]string{}
	inf := NewInformer(m, "/p/", WithInformerHandler(func(et InformerEventType, key string, v []byte) {
		if et == InformerEventPut {
			mu.Lock()
			seen[key] = string(v)
			mu.Unlock()
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForLen(t, inf.Store(), 2)
	cancel()
	<-errCh
	mu.Lock()
	defer mu.Unlock()
	if seen["/p/x"] != "vx" || seen["/p/y"] != "vy" {
		t.Errorf("handler not called for all keys: %v", seen)
	}
}

// TestInformer_WatchPutAppliesToStore Watch 产生的 Put 事件应更新 Store。
func TestInformer_WatchPutAppliesToStore(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{rev: 1})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	// 等待 Watch 已建立（wchCalls > 0）
	waitForCondition(t, func() bool { return m.wchCalls.Load() > 0 })
	m.sendPut("/p/new", "vnew", 10)
	waitForLen(t, inf.Store(), 1)
	v, _ := inf.Store().Get("/p/new")
	if string(v) != "vnew" {
		t.Errorf("v = %q", v)
	}
	cancel()
	<-errCh
}

// TestInformer_WatchDeleteAppliesToStore Watch 产生的 Delete 事件应从 Store 移除 key。
func TestInformer_WatchDeleteAppliesToStore(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{
		rev: 1,
		kvs: []*mvccpb.KeyValue{{Key: []byte("/p/old"), Value: []byte("v"), ModRevision: 1}},
	})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForLen(t, inf.Store(), 1)
	waitForCondition(t, func() bool { return m.wchCalls.Load() > 0 })
	m.sendDelete("/p/old", 5)
	waitForCondition(t, func() bool { return inf.Store().Len() == 0 })
	cancel()
	<-errCh
}

// TestInformer_ProgressNotifyIgnored IsProgressNotify 事件不应改变 Store。
func TestInformer_ProgressNotifyIgnored(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{rev: 1})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForCondition(t, func() bool { return m.wchCalls.Load() > 0 })
	m.sendProgressNotify()
	// 稍等让 observe 处理事件
	time.Sleep(50 * time.Millisecond)
	if inf.Store().Len() != 0 {
		t.Errorf("store should remain empty, got %d", inf.Store().Len())
	}
	cancel()
	<-errCh
}

// TestInformer_WatchChannelCloseTriggersReList Watch channel 关闭应触发 re-list。
func TestInformer_WatchChannelCloseTriggersReList(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{rev: 1}) // 初始 list
	m.queueGet(mockGetResp{         // re-list 响应
		rev: 10,
		kvs: []*mvccpb.KeyValue{{Key: []byte("/p/recovered"), Value: []byte("v"), ModRevision: 9}},
	})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForCondition(t, func() bool { return m.wchCalls.Load() > 0 })
	m.closeWatch()
	// re-list 后 store 应包含 recovered key
	waitForLen(t, inf.Store(), 1)
	cancel()
	<-errCh
	if m.getCalls.Load() < 2 {
		t.Errorf("expected >=2 Get calls (list + re-list), got %d", m.getCalls.Load())
	}
}

// TestInformer_InitialListError 首次 List 失败应直接返回错误。
func TestInformer_InitialListError(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	sentinel := errors.New("dial")
	m.queueGet(mockGetResp{err: sentinel})
	inf := NewInformer(m, "/p/")
	err := inf.Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("want wrapped sentinel, got %v", err)
	}
}

// TestInformer_HandlerPanicRecovered Handler panic 不应中断 Watch 循环。
func TestInformer_HandlerPanicRecovered(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{
		rev: 1,
		kvs: []*mvccpb.KeyValue{{Key: []byte("/p/k"), Value: []byte("v"), ModRevision: 1}},
	})
	var calls atomic.Int32
	inf := NewInformer(m, "/p/",
		WithInformerHandler(func(InformerEventType, string, []byte) {
			calls.Add(1)
			panic("boom")
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForLen(t, inf.Store(), 1)
	// Store 仍然被更新，handler 被调用（且 panic 被恢复）。
	if calls.Load() == 0 {
		t.Error("handler should have been called")
	}
	cancel()
	<-errCh
}

// TestInformer_ReListBackoffRetries 验证 re-list 失败→退避→下一轮成功路径。
// 每次 Watch 断开触发 1 次 list 尝试；为消耗多个 list 失败需要多次触发 Watch close。
func TestInformer_ReListBackoffRetries(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{rev: 1})                  // 初始 list
	m.queueGet(mockGetResp{err: errors.New("boom")}) // 第1次 re-list 失败
	m.queueGet(mockGetResp{rev: 10})                 // 第2次 re-list 成功
	inf := NewInformer(m, "/p/",
		WithInformerBackoff(5*time.Millisecond, 20*time.Millisecond),
	)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)

	waitForCondition(t, func() bool { return m.wchCalls.Load() >= 1 })
	m.closeWatch() // 触发首次 re-list（失败）
	waitForCondition(t, func() bool { return m.wchCalls.Load() >= 2 })
	m.closeWatch() // 触发第二次 re-list（成功，rev=10）
	waitForCondition(t, func() bool { return inf.Store().Rev() == 10 })
	cancel()
	<-errCh
	if m.getCalls.Load() < 3 {
		t.Errorf("expected >=3 Get calls (1 list + 2 re-list), got %d", m.getCalls.Load())
	}
}

// TestInformer_ContextCancelExitsCleanly ctx 取消应让 Run 返回 context.Canceled。
func TestInformer_ContextCancelExitsCleanly(t *testing.T) {
	t.Parallel()
	m := newMockListWatcher()
	m.queueGet(mockGetResp{rev: 1})
	inf := NewInformer(m, "/p/")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := runInBackground(t, inf, ctx)
	waitForCondition(t, func() bool { return m.wchCalls.Load() > 0 })
	cancel()
	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("want nil or Canceled, got %v", err)
	}
}

// TestInformer_OptionGuards 选项对非法输入保持默认。
func TestInformer_OptionGuards(t *testing.T) {
	t.Parallel()
	inf := NewInformer(newMockListWatcher(), "/p/",
		nil,
		WithInformerLogger(nil),
		WithInformerBackoff(0, 0),
	)
	if inf.retryBase != defaultInformerRetryBase {
		t.Errorf("retryBase changed: %v", inf.retryBase)
	}
	if inf.retryMaxWait != defaultInformerRetryMaxWait {
		t.Errorf("retryMaxWait changed: %v", inf.retryMaxWait)
	}
	if inf.logger == nil {
		t.Error("logger cleared by nil option")
	}
}

// TestInformer_BackoffOverride 合法区间应被采纳。
func TestInformer_BackoffOverride(t *testing.T) {
	t.Parallel()
	inf := NewInformer(newMockListWatcher(), "/p/",
		WithInformerBackoff(5*time.Millisecond, 100*time.Millisecond),
	)
	if inf.retryBase != 5*time.Millisecond || inf.retryMaxWait != 100*time.Millisecond {
		t.Errorf("backoff = (%v, %v)", inf.retryBase, inf.retryMaxWait)
	}
}

func waitForCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

// FuzzInformer_PrefixNoPanic 任意 prefix 字节串不应 panic。
func FuzzInformer_PrefixNoPanic(f *testing.F) {
	for _, s := range []string{"", "/", "/p/", "\x00", "中文"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, prefix string) {
		m := newMockListWatcher()
		m.queueGet(mockGetResp{rev: 1})
		inf := NewInformer(m, prefix)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if err := inf.Run(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			// 非 ctx 错误也不应 panic；此分支正常，返回 nil 即可
			_ = err
		}
	})
}
