//go:build integration

package xdlock_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// setupEtcd 启动 etcd 容器或连接到已有 etcd。
// 如果设置了 XKIT_ETCD_ENDPOINTS 环境变量，直接使用外部 etcd。
func setupEtcd(t *testing.T) (*clientv3.Client, func()) {
	t.Helper()

	// 优先使用环境变量指定的 etcd
	if endpoints := os.Getenv("XKIT_ETCD_ENDPOINTS"); endpoints != "" {
		client, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{endpoints},
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			t.Skipf("无法连接到 etcd %s: %v", endpoints, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := client.Status(ctx, endpoints); err != nil {
			_ = client.Close()
			t.Skipf("etcd 健康检查失败 %s: %v", endpoints, err)
		}

		return client, func() { _ = client.Close() }
	}

	// 使用 testcontainers 启动 etcd
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/coreos/etcd:v3.5.17",
		ExposedPorts: []string{"2379/tcp"},
		Cmd: []string{
			"etcd",
			"--advertise-client-urls=http://0.0.0.0:2379",
			"--listen-client-urls=http://0.0.0.0:2379",
		},
		WaitingFor: wait.ForLog("ready to serve client requests"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("无法启动 etcd 容器: %v", err)
	}

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("获取 etcd 端点失败: %v", err)
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://" + endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("创建 etcd 客户端失败: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		_ = container.Terminate(ctx)
	}

	return client, cleanup
}

// =============================================================================
// 工厂测试
// =============================================================================

func TestNewEtcdFactory_Success(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	assert.NotNil(t, factory.Session())
}

func TestEtcdFactory_WithTTL(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client, xdlock.WithEtcdTTL(10))
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	assert.NotNil(t, factory)
}

func TestEtcdFactory_Health(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = factory.Health(ctx)
	assert.NoError(t, err)
}

func TestEtcdFactory_HealthAfterClose(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)

	_ = factory.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = factory.Health(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestEtcdFactory_CloseIdempotent(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)

	// 多次关闭不应报错
	assert.NoError(t, factory.Close(context.Background()))
	assert.NoError(t, factory.Close(context.Background()))
}

// =============================================================================
// 锁基本操作测试（使用 Handle API）
// =============================================================================

func TestEtcdFactory_LockUnlock(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 获取锁
	handle, err := factory.Lock(ctx, "test-lock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 释放锁
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestEtcdFactory_TryLock_Success(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// TryLock 成功
	handle, err := factory.TryLock(ctx, "test-trylock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 验证 Key
	assert.Contains(t, handle.Key(), "test-trylock")

	// 释放锁
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestEtcdFactory_TryLock_LockHeld(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	// 创建两个工厂（两个不同的 Session）来测试真正的锁竞争
	factory1, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory1.Close() }()

	factory2, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory2.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 第一个工厂 TryLock 成功
	handle1, err := factory1.TryLock(ctx, "test-trylock-fail")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx) }()

	// 第二个工厂 TryLock 应该返回 (nil, nil) 表示锁被占用
	handle2, err := factory2.TryLock(ctx, "test-trylock-fail")
	assert.NoError(t, err)
	assert.Nil(t, handle2)
}

func TestEtcdLockHandle_Extend(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "test-extend")
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// etcd 使用 Session 自动续期，Extend 返回 nil
	err = handle.Extend(ctx)
	assert.NoError(t, err)
}

func TestEtcdFactory_WithKeyPrefix(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "mykey", xdlock.WithKeyPrefix("myapp:"))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// 验证 Key 包含前缀
	assert.Contains(t, handle.Key(), "mykey")
}

func TestEtcdFactory_Lock_ContextCanceled(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	// 创建两个工厂（两个不同的 Session）来测试真正的锁竞争
	factory1, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory1.Close() }()

	factory2, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory2.Close() }()

	// 第一个工厂持有锁
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	handle1, err := factory1.TryLock(ctx1, "test-ctx-cancel")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx1) }()

	// 第二个工厂 Lock 尝试获取锁，但 context 会被取消
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	handle2, err := factory2.Lock(ctx2, "test-ctx-cancel")
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Nil(t, handle2)
}

// =============================================================================
// 并发测试
// =============================================================================

func TestEtcdFactory_Lock_Concurrent(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 为每个 goroutine 创建独立的工厂（Session）以模拟真实的分布式锁竞争
	factories := make([]xdlock.EtcdFactory, goroutines)
	for i := 0; i < goroutines; i++ {
		factory, err := xdlock.NewEtcdFactory(client)
		require.NoError(t, err)
		factories[i] = factory
	}
	defer func() {
		for _, f := range factories {
			_ = f.Close(context.Background())
		}
	}()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()

			handle, err := factories[gid].Lock(ctx, "concurrent-lock")
			if err != nil {
				t.Logf("Lock failed: %v", err)
				return
			}
			defer func() { _ = handle.Unlock(ctx) }()

			// 临界区：递增计数器
			atomic.AddInt64(&counter, 1)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(goroutines), counter)
}

func TestEtcdFactory_Lock_MutualExclusion(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	const goroutines = 5
	const iterations = 10
	var counter int64
	var violations int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 为每个 goroutine 创建独立的工厂（Session）以模拟真实的分布式锁竞争
	factories := make([]xdlock.EtcdFactory, goroutines)
	for i := 0; i < goroutines; i++ {
		factory, err := xdlock.NewEtcdFactory(client)
		require.NoError(t, err)
		factories[i] = factory
	}
	defer func() {
		for _, f := range factories {
			_ = f.Close(context.Background())
		}
	}()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				handle, err := factories[gid].Lock(ctx, "mutual-exclusion")
				if err != nil {
					t.Logf("Lock failed: %v", err)
					continue
				}

				// 检查互斥性
				current := atomic.AddInt64(&counter, 1)
				if current != 1 {
					atomic.AddInt64(&violations, 1)
				}

				// 模拟工作
				time.Sleep(10 * time.Millisecond)

				atomic.AddInt64(&counter, -1)
				_ = handle.Unlock(ctx)
			}
		}(i)
	}

	wg.Wait()
	assert.Zero(t, violations, "mutex violation detected")
}

// =============================================================================
// Session 接口测试
// =============================================================================

func TestEtcdFactory_Session(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	session := factory.Session()
	assert.NotNil(t, session)
}
