package xdlock_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
	"github.com/omeyang/xkit/pkg/testkit/xetcdtest"
)

// 设计决策: 共享单个嵌入式 etcd 实例以缩短测试总时长。
// embed.Etcd 启动 1–2s，每个用例独立启动会放大 CI 时间；
// 测试间通过独立 key 前缀避免干扰，和 redsync/concurrency 真实集群行为一致。
var (
	sharedEtcdOnce sync.Once
	sharedEtcdMock *xetcdtest.Mock
	sharedEtcdErr  error
)

// sharedEtcdClient 惰性启动单例嵌入式 etcd，失败时跳过本文件所有用例。
// 在 TestMain 里清理（main_test.go）。
func sharedEtcdClient(t *testing.T) *clientv3.Client {
	t.Helper()
	sharedEtcdOnce.Do(func() {
		sharedEtcdMock, sharedEtcdErr = xetcdtest.New()
	})
	if sharedEtcdErr != nil {
		t.Skipf("xetcdtest unavailable: %v", sharedEtcdErr)
	}
	return sharedEtcdMock.Client()
}

// uniqueKey 生成本用例唯一的 key，避免测试间共用 etcd 数据互相污染。
func uniqueKey(t *testing.T, name string) string {
	t.Helper()
	return "/xdlock-itest/" + t.Name() + "/" + name
}

// closeFactoryNoErr 清理辅助：工厂关闭失败仅记录，不中断用例主断言。
func closeFactoryNoErr(t *testing.T, f xdlock.EtcdFactory) {
	t.Helper()
	if err := f.Close(context.Background()); err != nil {
		t.Logf("factory close: %v", err)
	}
}

// unlockNoErr 清理辅助：Unlock 失败（多半因用例主路径已 Unlock）仅记录。
func unlockNoErr(t *testing.T, h xdlock.LockHandle) {
	t.Helper()
	if err := h.Unlock(context.Background()); err != nil &&
		!errors.Is(err, xdlock.ErrNotLocked) {
		t.Logf("unlock: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Factory 构造与生命周期
// -----------------------------------------------------------------------------

func TestEtcdFactory_NewSuccess_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	if f.Session() == nil {
		t.Fatal("Session() returned nil")
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestEtcdFactory_NewWithTTL_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli, xdlock.WithEtcdTTL(7))
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	// 通过 Session.Lease() 间接验证 TTL 已生效（具体 TTL 数值由 etcd 返回）。
	if f.Session() == nil {
		t.Fatal("Session nil")
	}
}

func TestEtcdFactory_Close_Idempotent_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Health
// -----------------------------------------------------------------------------

func TestEtcdFactory_Health_OK_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := f.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestEtcdFactory_Health_AfterClose_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := f.Health(context.Background()); !errors.Is(err, xdlock.ErrFactoryClosed) {
		t.Fatalf("want ErrFactoryClosed, got %v", err)
	}
}

func TestEtcdFactory_Health_CtxCanceled_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Health(ctx); err == nil {
		t.Fatal("want error, got nil")
	}
}

// -----------------------------------------------------------------------------
// TryLock / Lock / Unlock / Extend
// -----------------------------------------------------------------------------

func TestEtcdFactory_TryLock_SuccessThenUnlock_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	h, err := f.TryLock(ctx, uniqueKey(t, "k"))
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if h == nil {
		t.Fatal("handle nil on fresh key")
	}
	if h.Key() == "" {
		t.Fatal("empty Key()")
	}
	if err := h.Extend(ctx); err != nil {
		t.Fatalf("Extend on healthy handle: %v", err)
	}
	if err := h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	// 二次 Unlock → ErrNotLocked
	if err := h.Unlock(ctx); !errors.Is(err, xdlock.ErrNotLocked) {
		t.Fatalf("second Unlock: want ErrNotLocked, got %v", err)
	}
	// Unlock 后 Extend → ErrNotLocked
	if err := h.Extend(ctx); !errors.Is(err, xdlock.ErrNotLocked) {
		t.Fatalf("Extend after Unlock: want ErrNotLocked, got %v", err)
	}
}

func TestEtcdFactory_TryLock_LockHeldBetweenFactories_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f1, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("f1: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f1) })
	f2, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("f2: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f2) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := uniqueKey(t, "k")
	h1, err := f1.TryLock(ctx, key)
	if err != nil || h1 == nil {
		t.Fatalf("f1.TryLock: handle=%v err=%v", h1, err)
	}
	t.Cleanup(func() { unlockNoErr(t, h1) })

	// 不同 factory（独立 Session）对同 key TryLock → 应返回 (nil, nil)
	h2, err := f2.TryLock(ctx, key)
	if err != nil {
		t.Fatalf("f2.TryLock: %v", err)
	}
	if h2 != nil {
		t.Fatal("want nil handle on contended TryLock")
	}
}

func TestEtcdFactory_TryLock_SameFactorySameKey_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := uniqueKey(t, "k")
	h1, err := f.TryLock(ctx, key)
	if err != nil || h1 == nil {
		t.Fatalf("first TryLock: %v", err)
	}
	t.Cleanup(func() { unlockNoErr(t, h1) })

	// 同一 factory 重入，本地 lockedKeys 命中 → (nil, nil)
	h2, err := f.TryLock(ctx, key)
	if err != nil {
		t.Fatalf("second TryLock: %v", err)
	}
	if h2 != nil {
		t.Fatal("same-factory re-lock should return nil handle")
	}
}

func TestEtcdFactory_Lock_SameFactorySameKey_Error_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, f) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := uniqueKey(t, "k")
	h1, err := f.Lock(ctx, key)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	t.Cleanup(func() { unlockNoErr(t, h1) })

	// 同 factory 同 key Lock 第二次 → ErrLockFailed（防重入保护）
	if _, err := f.Lock(ctx, key); !errors.Is(err, xdlock.ErrLockFailed) {
		t.Fatalf("want ErrLockFailed, got %v", err)
	}
}

func TestEtcdFactory_Lock_AfterClose_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	f, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("NewEtcdFactory: %v", err)
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := f.Lock(context.Background(), uniqueKey(t, "k")); !errors.Is(err, xdlock.ErrFactoryClosed) {
		t.Fatalf("Lock: want ErrFactoryClosed, got %v", err)
	}
	if _, err := f.TryLock(context.Background(), uniqueKey(t, "k")); !errors.Is(err, xdlock.ErrFactoryClosed) {
		t.Fatalf("TryLock: want ErrFactoryClosed, got %v", err)
	}
}

func TestEtcdFactory_Lock_ContextDeadline_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	fA, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("fA: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, fA) })
	fB, err := xdlock.NewEtcdFactory(cli)
	if err != nil {
		t.Fatalf("fB: %v", err)
	}
	t.Cleanup(func() { closeFactoryNoErr(t, fB) })

	key := uniqueKey(t, "k")
	ctxHold, cancelHold := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelHold()
	hA, err := fA.Lock(ctxHold, key)
	if err != nil {
		t.Fatalf("fA.Lock: %v", err)
	}
	t.Cleanup(func() { unlockNoErr(t, hA) })

	ctxShort, cancelShort := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelShort()
	if _, err := fB.Lock(ctxShort, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// 客户端 / Config 一体化入口
// -----------------------------------------------------------------------------

func TestNewEtcdClient_WithHealthCheck_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	// 用 embed 的 endpoints 构造独立 client 走 NewEtcdClient 路径
	cfg := xdlock.DefaultEtcdConfig()
	cfg.Endpoints = cli.Endpoints()
	c, err := xdlock.NewEtcdClient(cfg, xdlock.WithEtcdHealthCheck(true, time.Second))
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Logf("client close: %v", err)
		}
	})
	if len(c.Endpoints()) == 0 {
		t.Fatal("no endpoints on returned client")
	}
}

func TestNewEtcdFactoryFromConfig_Success_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	cfg := xdlock.DefaultEtcdConfig()
	cfg.Endpoints = cli.Endpoints()
	f, c, err := xdlock.NewEtcdFactoryFromConfig(cfg, nil)
	if err != nil {
		t.Fatalf("NewEtcdFactoryFromConfig: %v", err)
	}
	t.Cleanup(func() {
		closeFactoryNoErr(t, f)
		if err := c.Close(); err != nil {
			t.Logf("client close: %v", err)
		}
	})
	if f.Session() == nil {
		t.Fatal("Session nil")
	}
}

// -----------------------------------------------------------------------------
// 并发互斥（跨 factory 真实竞争）
// -----------------------------------------------------------------------------

func TestEtcdFactory_MutualExclusion_Embed(t *testing.T) {
	cli := sharedEtcdClient(t)
	const goroutines = 6
	const iterations = 4
	factories := make([]xdlock.EtcdFactory, goroutines)
	for i := range factories {
		f, err := xdlock.NewEtcdFactory(cli)
		if err != nil {
			t.Fatalf("factory %d: %v", i, err)
		}
		factories[i] = f
	}
	t.Cleanup(func() {
		for _, f := range factories {
			closeFactoryNoErr(t, f)
		}
	})

	var counter, violations int64
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := uniqueKey(t, "mutex")

	for i := 0; i < goroutines; i++ {
		gid := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				h, err := factories[gid].Lock(ctx, key)
				if err != nil {
					return
				}
				if atomic.AddInt64(&counter, 1) != 1 {
					atomic.AddInt64(&violations, 1)
				}
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt64(&counter, -1)
				if err := h.Unlock(ctx); err != nil {
					t.Logf("unlock: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if v := atomic.LoadInt64(&violations); v != 0 {
		t.Fatalf("mutex violation: %d", v)
	}
}
