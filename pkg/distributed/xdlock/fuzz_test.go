package xdlock_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// =============================================================================
// Fuzz 测试辅助函数
// =============================================================================

// setupMultipleMiniredis 创建 n 个 miniredis 实例和对应的客户端。
// skipIndices 中指定的索引位置将保持 nil（用于测试 nil 客户端）。
// 返回 false 表示资源不足，调用方应跳过。
func setupMultipleMiniredis(
	n int,
	skipIndices map[int]struct{},
) ([]redis.UniversalClient, []*miniredis.Miniredis, bool) {
	clients := make([]redis.UniversalClient, n)
	mrs := make([]*miniredis.Miniredis, n)

	for i := 0; i < n; i++ {
		if _, skip := skipIndices[i]; skip {
			continue
		}

		mr, err := miniredis.Run()
		if err != nil {
			cleanupMiniredisInstances(clients[:i], mrs[:i])
			return nil, nil, false
		}
		mrs[i] = mr
		clients[i] = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	}

	return clients, mrs, true
}

// cleanupMiniredisInstances 关闭所有客户端和 miniredis 实例。
func cleanupMiniredisInstances(clients []redis.UniversalClient, mrs []*miniredis.Miniredis) {
	for i := range clients {
		if clients[i] != nil {
			_ = clients[i].Close()
		}
		if mrs[i] != nil {
			mrs[i].Close()
		}
	}
}

// setupFuzzRedis 创建单个 miniredis + 客户端 + 工厂。
// 返回 false 表示资源不足，调用方应跳过。
func setupFuzzRedis() (
	*miniredis.Miniredis, redis.UniversalClient, xdlock.RedisFactory, bool,
) {
	mr, err := miniredis.Run()
	if err != nil {
		return nil, nil, nil, false
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		_ = client.Close()
		mr.Close()
		return nil, nil, nil, false
	}

	return mr, client, factory, true
}

// executeLockOps 执行操作序列（0=TryLock, 1=Lock, 2=Unlock, 3=Extend）。
// 返回最终持有的 LockHandle（可能为 nil）。
func executeLockOps(
	ctx context.Context,
	factory xdlock.RedisFactory,
	key string,
	ops []byte,
	lockOpts []xdlock.MutexOption,
) xdlock.LockHandle {
	var current xdlock.LockHandle

	for _, op := range ops {
		current = executeSingleLockOp(ctx, factory, key, op%4, current, lockOpts)
	}

	return current
}

// executeSingleLockOp 执行单个锁操作，返回更新后的当前 handle。
func executeSingleLockOp(
	ctx context.Context,
	factory xdlock.RedisFactory,
	key string,
	op byte,
	current xdlock.LockHandle,
	lockOpts []xdlock.MutexOption,
) xdlock.LockHandle {
	switch op {
	case 0: // TryLock
		return tryAcquireLock(ctx, key, current, lockOpts, factory.TryLock)
	case 1: // Lock
		return tryAcquireLock(ctx, key, current, lockOpts, factory.Lock)
	case 2: // Unlock
		if current != nil {
			_ = current.Unlock(ctx)
			return nil
		}
	case 3: // Extend
		if current != nil {
			_ = current.Extend(ctx)
		}
	}

	return current
}

// lockFunc 是 TryLock/Lock 的统一签名。
type lockFunc func(context.Context, string, ...xdlock.MutexOption) (xdlock.LockHandle, error)

// tryAcquireLock 尝试获取锁，如果成功则释放旧 handle。
func tryAcquireLock(
	ctx context.Context,
	key string,
	current xdlock.LockHandle,
	lockOpts []xdlock.MutexOption,
	acquire lockFunc,
) xdlock.LockHandle {
	handle, err := acquire(ctx, key, lockOpts...)
	if err != nil || handle == nil {
		return current
	}
	if current != nil {
		_ = current.Unlock(ctx)
	}
	return handle
}

// =============================================================================
// 工厂创建 Fuzz 测试
// =============================================================================

// FuzzNewRedisFactory 测试 Redis 工厂创建的鲁棒性。
func FuzzNewRedisFactory(f *testing.F) {
	// 种子：测试 nil 和有效客户端
	f.Add(0) // 无客户端
	f.Add(1) // 单客户端
	f.Add(3) // 多客户端（Redlock）

	f.Fuzz(func(t *testing.T, numClients int) {
		if numClients < 0 || numClients > 10 {
			return
		}

		if numClients == 0 {
			_, err := xdlock.NewRedisFactory()
			if err == nil {
				t.Error("expected error for no clients")
			}
			return
		}

		clients, mrs, ok := setupMultipleMiniredis(numClients, nil)
		if !ok {
			return
		}
		defer cleanupMiniredisInstances(clients, mrs)

		factory, err := xdlock.NewRedisFactory(clients...)
		if err != nil {
			t.Errorf("unexpected error creating factory: %v", err)
			return
		}
		defer func() { _ = factory.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		if err := factory.Health(ctx); err != nil {
			t.Errorf("health check failed: %v", err)
		}
	})
}

// FuzzNewRedisFactory_NilClients 测试包含 nil 客户端的情况。
func FuzzNewRedisFactory_NilClients(f *testing.F) {
	// 种子：nil 位置
	f.Add(0, 3)  // 第一个是 nil
	f.Add(1, 3)  // 中间是 nil
	f.Add(2, 3)  // 最后一个是 nil
	f.Add(-1, 3) // 全部是 nil

	f.Fuzz(func(t *testing.T, nilIndex, total int) {
		if total < 1 || total > 5 {
			return
		}

		skipIndices := buildNilIndices(nilIndex, total)
		clients, mrs, ok := setupMultipleMiniredis(total, skipIndices)
		if !ok {
			return
		}
		defer cleanupMiniredisInstances(clients, mrs)

		// 包含 nil 客户端应该返回错误
		_, err := xdlock.NewRedisFactory(clients...)
		if err == nil {
			t.Error("expected error for nil client")
		}
	})
}

// buildNilIndices 根据 nilIndex 和 total 构建需要跳过的索引集合。
// nilIndex == -1 表示全部跳过，否则跳过 nilIndex%total 位置。
func buildNilIndices(nilIndex, total int) map[int]struct{} {
	skip := make(map[int]struct{})
	if nilIndex == -1 {
		for i := 0; i < total; i++ {
			skip[i] = struct{}{}
		}
	} else {
		skip[nilIndex%total] = struct{}{}
	}
	return skip
}

// =============================================================================
// TryLock Key 名称 Fuzz 测试
// =============================================================================

// FuzzTryLock_KeyName 测试各种 key 名称的处理。
func FuzzTryLock_KeyName(f *testing.F) {
	// 种子语料库
	// 有效值
	f.Add("my-lock")
	f.Add("lock_123")
	f.Add("resource.lock")
	f.Add("a")

	// 边界值
	f.Add("")
	f.Add(" ")
	f.Add("   ")

	// 特殊字符
	f.Add("lock:key")
	f.Add("lock/path/to/resource")
	f.Add("lock\x00null")
	f.Add("中文锁名")
	f.Add("キー")
	f.Add("🔒")

	// 长字符串
	f.Add(strings.Repeat("x", 100))
	f.Add(strings.Repeat("a", 1000))

	f.Fuzz(func(t *testing.T, key string) {
		// 限制 key 长度
		if len(key) > 10000 {
			return
		}

		mr, err := miniredis.Run()
		if err != nil {
			return
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client.Close() }()

		factory, err := xdlock.NewRedisFactory(client)
		if err != nil {
			t.Fatalf("failed to create factory: %v", err)
		}
		defer func() { _ = factory.Close() }()

		// 对于非空 key，尝试获取和释放锁
		if key != "" && len(key) < 1000 {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// TryLock 不应该 panic
			handle, err := factory.TryLock(ctx, key)
			if err == nil && handle != nil {
				// 成功获取锁，释放它
				_ = handle.Unlock(ctx)
			}
			// 错误时不报告，因为某些特殊 key 可能导致 Redis 错误
		}
	})
}

// =============================================================================
// 选项参数 Fuzz 测试
// =============================================================================

// FuzzWithKeyPrefix 测试 key 前缀选项。
func FuzzWithKeyPrefix(f *testing.F) {
	f.Add("")
	f.Add("lock:")
	f.Add("myapp:")
	f.Add("a/b/c/")
	f.Add(strings.Repeat("prefix:", 100))
	f.Add("中文前缀:")

	f.Fuzz(func(t *testing.T, prefix string) {
		if len(prefix) > 10000 {
			return
		}

		// 选项函数不应该 panic
		opt := xdlock.WithKeyPrefix(prefix)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithExpiry 测试过期时间选项。
func FuzzWithExpiry(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(time.Second))
	f.Add(int64(time.Minute))
	f.Add(int64(time.Hour))
	f.Add(int64(-1))
	f.Add(int64(-time.Second))
	f.Add(int64(1<<62 - 1)) // 接近 max int64

	f.Fuzz(func(t *testing.T, expiryNs int64) {
		expiry := time.Duration(expiryNs)

		// 选项函数不应该 panic
		opt := xdlock.WithExpiry(expiry)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithTries 测试重试次数选项。
func FuzzWithTries(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(5)
	f.Add(32)
	f.Add(100)
	f.Add(-1)
	f.Add(-100)
	f.Add(1 << 30) // 大值

	f.Fuzz(func(t *testing.T, tries int) {
		// 选项函数不应该 panic
		opt := xdlock.WithTries(tries)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithRetryDelay 测试重试延迟选项。
func FuzzWithRetryDelay(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(time.Millisecond))
	f.Add(int64(100 * time.Millisecond))
	f.Add(int64(time.Second))
	f.Add(int64(-1))
	f.Add(int64(-time.Second))

	f.Fuzz(func(t *testing.T, delayNs int64) {
		delay := time.Duration(delayNs)

		opt := xdlock.WithRetryDelay(delay)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithDriftFactor 测试漂移因子选项。
func FuzzWithDriftFactor(f *testing.F) {
	f.Add(0.0)
	f.Add(0.01)
	f.Add(0.1)
	f.Add(1.0)
	f.Add(-0.01)
	f.Add(-1.0)
	f.Add(1e308)  // 接近 max float64
	f.Add(-1e308) // 接近 min float64

	f.Fuzz(func(t *testing.T, factor float64) {
		// 跳过 NaN 和 Inf
		if factor != factor { // NaN check
			return
		}

		opt := xdlock.WithDriftFactor(factor)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithTimeoutFactor 测试超时因子选项。
func FuzzWithTimeoutFactor(f *testing.F) {
	f.Add(0.0)
	f.Add(0.05)
	f.Add(0.1)
	f.Add(1.0)
	f.Add(-0.05)

	f.Fuzz(func(t *testing.T, factor float64) {
		if factor != factor { // NaN check
			return
		}

		opt := xdlock.WithTimeoutFactor(factor)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzTryLock_CombinedOptions 测试组合选项。
func FuzzTryLock_CombinedOptions(f *testing.F) {
	f.Add("prefix:", int64(time.Second), 5, int64(100*time.Millisecond), 0.01, 0.05, true, false)

	f.Fuzz(func(t *testing.T,
		prefix string,
		expiryNs int64,
		tries int,
		delayNs int64,
		driftFactor float64,
		timeoutFactor float64,
		failFast bool,
		shufflePools bool,
	) {
		// 限制参数范围
		if len(prefix) > 1000 {
			return
		}
		if driftFactor != driftFactor || timeoutFactor != timeoutFactor {
			return // NaN
		}

		mr, err := miniredis.Run()
		if err != nil {
			return
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client.Close() }()

		factory, err := xdlock.NewRedisFactory(client)
		if err != nil {
			return
		}
		defer func() { _ = factory.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// 使用所有选项创建锁不应该 panic
		opts := []xdlock.MutexOption{
			xdlock.WithKeyPrefix(prefix),
			xdlock.WithExpiry(time.Duration(expiryNs)),
			xdlock.WithTries(tries),
			xdlock.WithRetryDelay(time.Duration(delayNs)),
			xdlock.WithDriftFactor(driftFactor),
			xdlock.WithTimeoutFactor(timeoutFactor),
			xdlock.WithFailFast(failFast),
			xdlock.WithShufflePools(shufflePools),
		}

		handle, err := factory.TryLock(ctx, "test-key", opts...)
		if err == nil && handle != nil {
			_ = handle.Unlock(ctx)
		}
	})
}

// =============================================================================
// etcd 工厂选项 Fuzz 测试
// =============================================================================

// FuzzWithEtcdTTL 测试 etcd TTL 选项。
func FuzzWithEtcdTTL(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(60)
	f.Add(300)
	f.Add(-1)
	f.Add(-60)
	f.Add(1 << 30)

	f.Fuzz(func(t *testing.T, ttl int) {
		opt := xdlock.WithEtcdTTL(ttl)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// FuzzWithEtcdContext 测试 etcd 上下文选项。
func FuzzWithEtcdContext(f *testing.F) {
	f.Add(true)  // 使用 context.Background()
	f.Add(false) // 使用 nil context

	f.Fuzz(func(t *testing.T, useValidContext bool) {
		var ctx context.Context
		if useValidContext {
			ctx = context.Background()
		}

		opt := xdlock.WithEtcdContext(ctx)
		if opt == nil {
			t.Error("option should not be nil")
		}
	})
}

// =============================================================================
// 锁操作 Fuzz 测试
// =============================================================================

// FuzzLockHandle_Operations 测试锁操作的鲁棒性。
func FuzzLockHandle_Operations(f *testing.F) {
	// ops: 0=TryLock, 1=Lock, 2=Unlock, 3=Extend
	f.Add("key", []byte{0, 2})       // TryLock + Unlock
	f.Add("key", []byte{1, 2})       // Lock + Unlock
	f.Add("key", []byte{0, 3, 2})    // TryLock + Extend + Unlock
	f.Add("key", []byte{2})          // Unlock without lock (no-op)
	f.Add("key", []byte{3})          // Extend without lock (no-op)
	f.Add("key", []byte{0, 0})       // Double TryLock
	f.Add("key", []byte{0, 2, 2})    // Double Unlock
	f.Add("key", []byte{})           // No operations
	f.Add("key", []byte{0, 2, 0, 2}) // TryLock + Unlock + TryLock + Unlock

	f.Fuzz(func(t *testing.T, key string, ops []byte) {
		if len(key) == 0 || len(key) > 100 || len(ops) > 20 {
			return
		}

		mr, client, factory, ok := setupFuzzRedis()
		if !ok {
			return
		}
		defer mr.Close()
		defer func() { _ = client.Close() }()
		defer func() { _ = factory.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		lockOpts := []xdlock.MutexOption{
			xdlock.WithExpiry(time.Second),
			xdlock.WithTries(1),
		}

		// 执行操作序列，不应该 panic
		currentHandle := executeLockOps(ctx, factory, key, ops, lockOpts)

		// 清理
		if currentHandle != nil {
			_ = currentHandle.Unlock(ctx)
		}
	})
}

// FuzzTryLock_ContextTimeout 测试不同超时值的处理。
func FuzzTryLock_ContextTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(time.Millisecond))
	f.Add(int64(10 * time.Millisecond))
	f.Add(int64(100 * time.Millisecond))
	f.Add(int64(time.Second))
	f.Add(int64(-1)) // 负值

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		// 限制超时范围
		if timeoutNs < 0 {
			timeoutNs = 0
		}
		if timeoutNs > int64(time.Second) {
			timeoutNs = int64(time.Second)
		}

		mr, err := miniredis.Run()
		if err != nil {
			return
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client.Close() }()

		factory, err := xdlock.NewRedisFactory(client)
		if err != nil {
			return
		}
		defer func() { _ = factory.Close() }()

		timeout := time.Duration(timeoutNs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// 操作可能超时或成功，但不应该 panic
		handle, err := factory.TryLock(ctx, "timeout-test", xdlock.WithTries(1))
		if err == nil && handle != nil {
			_ = handle.Unlock(context.Background())
		}
	})
}

// =============================================================================
// 错误处理 Fuzz 测试
// =============================================================================

// FuzzFactoryClose_Operations 测试工厂关闭后的操作。
func FuzzFactoryClose_Operations(f *testing.F) {
	f.Add(true, true)   // Close 后 Health
	f.Add(true, false)  // Close 后 TryLock
	f.Add(false, true)  // 不 Close
	f.Add(false, false) // 不 Close

	f.Fuzz(func(t *testing.T, closeFirst, checkHealth bool) {
		mr, err := miniredis.Run()
		if err != nil {
			return
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer func() { _ = client.Close() }()

		factory, err := xdlock.NewRedisFactory(client)
		if err != nil {
			return
		}

		if closeFirst {
			_ = factory.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		if checkHealth {
			err := factory.Health(ctx)
			if closeFirst && err == nil {
				t.Error("expected error after close")
			}
		} else {
			handle, _ := factory.TryLock(ctx, "test")
			if handle != nil {
				_ = handle.Unlock(ctx)
			}
		}

		if !closeFirst {
			_ = factory.Close()
		}
	})
}
