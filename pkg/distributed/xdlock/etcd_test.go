//go:build integration

package xdlock_test

import (
	"context"
	"errors"
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

// setupEtcd 启动 etcd 容器并返回客户端。
func setupEtcd(t *testing.T) (*clientv3.Client, func()) {
	t.Helper()

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
	require.NoError(t, err, "failed to start etcd container")

	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err, "failed to get etcd endpoint")

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://" + endpoint},
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err, "failed to create etcd client")

	cleanup := func() {
		client.Close()
		container.Terminate(ctx)
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
	defer factory.Close()

	assert.NotNil(t, factory.Session())
}

func TestEtcdFactory_WithTTL(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client, xdlock.WithEtcdTTL(10))
	require.NoError(t, err)
	defer factory.Close()

	assert.NotNil(t, factory)
}

func TestEtcdFactory_Health(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

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

	factory.Close()

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
	assert.NoError(t, factory.Close())
	assert.NoError(t, factory.Close())
}

// =============================================================================
// 锁基本操作测试
// =============================================================================

func TestEtcdLocker_LockUnlock(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-lock")

	// 获取锁
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 释放锁
	err = locker.Unlock(ctx)
	assert.NoError(t, err)
}

func TestEtcdLocker_TryLock(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-trylock")

	// TryLock 成功
	err = locker.TryLock(ctx)
	require.NoError(t, err)

	// 释放锁
	err = locker.Unlock(ctx)
	assert.NoError(t, err)
}

func TestEtcdLocker_TryLockFailed(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 第一个 locker 获取锁
	locker1 := factory.NewMutex("test-trylock-fail")
	err = locker1.Lock(ctx)
	require.NoError(t, err)
	defer locker1.Unlock(ctx)

	// 第二个 locker TryLock 应该失败
	locker2 := factory.NewMutex("test-trylock-fail")
	err = locker2.TryLock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockHeld)
}

func TestEtcdLocker_Extend(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-extend")
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer locker.Unlock(ctx)

	// etcd 不支持手动续期
	err = locker.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrExtendNotSupported)
}

func TestEtcdLocker_WithKeyPrefix(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("mykey", xdlock.WithKeyPrefix("myapp:"))

	// 验证 key 前缀
	etcdLocker, ok := locker.(xdlock.EtcdLocker)
	require.True(t, ok)
	assert.Equal(t, "myapp:mykey", etcdLocker.Key())

	// 正常获取和释放锁
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer locker.Unlock(ctx)
}

func TestEtcdLocker_ContextCanceled(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	// 第一个 locker 持有锁
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	locker1 := factory.NewMutex("test-ctx-cancel")
	err = locker1.Lock(ctx1)
	require.NoError(t, err)
	defer locker1.Unlock(ctx1)

	// 第二个 locker 尝试获取锁，但 context 会被取消
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	locker2 := factory.NewMutex("test-ctx-cancel")
	err = locker2.Lock(ctx2)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

// =============================================================================
// 并发测试
// =============================================================================

func TestEtcdLocker_Concurrent(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			locker := factory.NewMutex("concurrent-lock")
			if err := locker.Lock(ctx); err != nil {
				t.Logf("Lock failed: %v", err)
				return
			}
			defer locker.Unlock(ctx)

			// 临界区：递增计数器
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(goroutines), counter)
}

func TestEtcdLocker_MutualExclusion(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	const goroutines = 5
	const iterations = 10
	var counter int64
	var violations int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				locker := factory.NewMutex("mutual-exclusion")
				if err := locker.Lock(ctx); err != nil {
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
				locker.Unlock(ctx)
			}
		}()
	}

	wg.Wait()
	assert.Zero(t, violations, "mutex violation detected")
}

// =============================================================================
// 接口实现验证
// =============================================================================

func TestEtcdLocker_ImplementsEtcdLocker(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	factory, err := xdlock.NewEtcdFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	locker := factory.NewMutex("test")
	etcdLocker, ok := locker.(xdlock.EtcdLocker)
	assert.True(t, ok)
	assert.NotNil(t, etcdLocker.Mutex())
	assert.Contains(t, etcdLocker.Key(), "test")
}
