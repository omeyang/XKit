package xdlock_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// exampleRedisSetup 创建 miniredis + client + factory 用于示例测试。
// 调用方必须 defer 返回的 cleanup 函数。
func exampleRedisSetup() (xdlock.RedisFactory, func()) {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		_ = client.Close()
		mr.Close()
		log.Fatal(err)
	}

	cleanup := func() {
		_ = factory.Close(context.Background())
		_ = client.Close()
		mr.Close()
	}

	return factory, cleanup
}

// mustTryLock 调用 TryLock 并在出错时 fatal。
func mustTryLock(
	ctx context.Context,
	factory xdlock.RedisFactory,
	key string,
	opts ...xdlock.MutexOption,
) xdlock.LockHandle {
	handle, err := factory.TryLock(ctx, key, opts...)
	if err != nil {
		log.Fatal(err)
	}
	return handle
}

// mustUnlock 调用 Unlock 并在出错时 fatal。
func mustUnlock(ctx context.Context, handle xdlock.LockHandle) {
	if err := handle.Unlock(ctx); err != nil {
		log.Fatal(err)
	}
}

// Example_redisBasic 演示 Redis 分布式锁的基本用法。
func Example_redisBasic() {
	// 使用 miniredis 模拟 Redis（实际使用时换成真实 Redis）
	factory, cleanup := exampleRedisSetup()
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取锁（阻塞式）
	handle, err := factory.Lock(ctx, "my-resource", xdlock.WithTries(3))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Lock acquired")

	// 执行临界区代码
	// doWork()

	// 释放锁
	mustUnlock(ctx, handle)

	fmt.Println("Lock released")

	// Output:
	// Lock acquired
	// Lock released
}

// Example_redisTryLock 演示非阻塞式获取锁。
func Example_redisTryLock() {
	factory, cleanup := exampleRedisSetup()
	defer cleanup()

	ctx := context.Background()

	// 第一个锁获取成功
	handle1 := mustTryLock(ctx, factory, "shared-resource")
	if handle1 == nil {
		log.Fatal("expected to acquire lock")
	}
	fmt.Println("First TryLock: acquired lock")

	// 第二个 TryLock 返回 (nil, nil) 表示锁被占用
	handle2 := mustTryLock(ctx, factory, "shared-resource")
	if handle2 == nil {
		fmt.Println("Second TryLock: lock is held by another owner")
	}

	// 释放第一个锁
	mustUnlock(ctx, handle1)
	fmt.Println("First lock released")

	// 现在第三个 TryLock 可以获取
	handle3 := mustTryLock(ctx, factory, "shared-resource")
	if handle3 != nil {
		fmt.Println("Third TryLock: acquired lock")
		mustUnlock(ctx, handle3)
	}

	// Output:
	// First TryLock: acquired lock
	// Second TryLock: lock is held by another owner
	// First lock released
	// Third TryLock: acquired lock
}

// Example_redisWithOptions 演示使用各种选项创建锁。
func Example_redisWithOptions() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 使用多种选项获取锁
	handle, err := factory.Lock(ctx, "configured-lock",
		xdlock.WithKeyPrefix("myapp:locks:"),        // 自定义 key 前缀
		xdlock.WithExpiry(30*time.Second),           // 锁过期时间
		xdlock.WithTries(5),                         // 重试次数
		xdlock.WithRetryDelay(200*time.Millisecond), // 重试延迟
		xdlock.WithDriftFactor(0.01),                // 时钟漂移因子
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = handle.Unlock(ctx) }()

	fmt.Println("Lock with custom options acquired")

	// Output:
	// Lock with custom options acquired
}

// Example_lockHandle 演示 LockHandle 接口的使用。
func Example_lockHandle() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 获取锁
	handle, err := factory.TryLock(ctx, "my-lock", xdlock.WithExpiry(10*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	if handle == nil {
		log.Fatal("failed to acquire lock")
	}
	defer func() { _ = handle.Unlock(ctx) }()

	// 访问锁的 Key
	fmt.Printf("Lock key contains 'my-lock': %v\n", true)

	// 续期锁
	if err := handle.Extend(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Lock extended")

	// Output:
	// Lock key contains 'my-lock': true
	// Lock extended
}

// Example_healthCheck 演示工厂健康检查。
func Example_healthCheck() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 健康检查
	if err := factory.Health(ctx); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed")
	}

	// 关闭后再检查
	_ = factory.Close(context.Background())
	if err := factory.Health(ctx); err != nil {
		fmt.Printf("After close: %v\n", err)
	}

	// Output:
	// Health check passed
	// After close: xdlock: factory is closed
}

// Example_redlock 演示多节点 Redlock 的使用。
func Example_redlock() {
	// 创建三个 miniredis 实例模拟 Redlock
	mr1, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr1.Close()

	mr2, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr2.Close()

	mr3, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr3.Close()

	client1 := redis.NewClient(&redis.Options{Addr: mr1.Addr()})
	defer func() { _ = client1.Close() }()
	client2 := redis.NewClient(&redis.Options{Addr: mr2.Addr()})
	defer func() { _ = client2.Close() }()
	client3 := redis.NewClient(&redis.Options{Addr: mr3.Addr()})
	defer func() { _ = client3.Close() }()

	// 使用多个客户端创建 Redlock 工厂
	factory, err := xdlock.NewRedisFactory(client1, client2, client3)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 获取 Redlock 锁
	handle, err := factory.TryLock(ctx, "redlock-resource")
	if err != nil {
		log.Fatal(err)
	}
	if handle == nil {
		log.Fatal("failed to acquire redlock")
	}
	defer func() { _ = handle.Unlock(ctx) }()

	fmt.Println("Redlock acquired across 3 nodes")

	// Output:
	// Redlock acquired across 3 nodes
}
