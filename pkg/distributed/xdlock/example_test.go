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

// Example_redisBasic 演示 Redis 分布式锁的基本用法。
func Example_redisBasic() {
	// 使用 miniredis 模拟 Redis（实际使用时换成真实 Redis）
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// 创建锁工厂
	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建锁实例
	locker := factory.NewMutex("my-resource", xdlock.WithTries(3))

	// 获取锁
	if err := locker.Lock(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Lock acquired")

	// 执行临界区代码
	// doWork()

	// 释放锁
	if err := locker.Unlock(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Lock released")

	// Output:
	// Lock acquired
	// Lock released
}

// Example_redisTryLock 演示非阻塞式获取锁。
func Example_redisTryLock() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer factory.Close()

	ctx := context.Background()

	// 第一个锁获取成功
	locker1 := factory.NewMutex("shared-resource", xdlock.WithTries(1))
	if err := locker1.TryLock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Locker1: acquired lock")

	// 第二个锁获取失败（锁被占用）
	locker2 := factory.NewMutex("shared-resource", xdlock.WithTries(1))
	if err := locker2.TryLock(ctx); err != nil {
		fmt.Printf("Locker2: %v\n", err)
	}

	// 释放第一个锁
	if err := locker1.Unlock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Locker1: released lock")

	// 现在第二个锁可以获取
	if err := locker2.TryLock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Locker2: acquired lock")

	if err := locker2.Unlock(ctx); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Locker1: acquired lock
	// Locker2: xdlock: lock is held by another owner
	// Locker1: released lock
	// Locker2: acquired lock
}

// Example_redisWithOptions 演示使用各种选项创建锁。
func Example_redisWithOptions() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer factory.Close()

	ctx := context.Background()

	// 使用多种选项创建锁
	locker := factory.NewMutex("configured-lock",
		xdlock.WithKeyPrefix("myapp:locks:"),        // 自定义 key 前缀
		xdlock.WithExpiry(30*time.Second),           // 锁过期时间
		xdlock.WithTries(5),                         // 重试次数
		xdlock.WithRetryDelay(200*time.Millisecond), // 重试延迟
		xdlock.WithDriftFactor(0.01),                // 时钟漂移因子
	)

	if err := locker.Lock(ctx); err != nil {
		log.Fatal(err)
	}
	defer locker.Unlock(ctx)

	fmt.Println("Lock with custom options acquired")

	// Output:
	// Lock with custom options acquired
}

// Example_redisLocker 演示获取底层 RedisLocker 接口。
func Example_redisLocker() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer factory.Close()

	ctx := context.Background()

	locker := factory.NewMutex("my-lock", xdlock.WithTries(1))

	if err := locker.Lock(ctx); err != nil {
		log.Fatal(err)
	}
	defer locker.Unlock(ctx)

	// 类型断言获取 RedisLocker 接口
	redisLocker, ok := locker.(xdlock.RedisLocker)
	if !ok {
		log.Fatal("not a RedisLocker")
	}

	// 访问 Redis 锁特有的方法
	fmt.Printf("Lock value length: %d\n", len(redisLocker.Value()))
	fmt.Printf("Lock has expiry: %v\n", redisLocker.Until() > 0)

	// 获取底层 redsync.Mutex（可用于高级操作）
	mutex := redisLocker.RedisMutex()
	fmt.Printf("Mutex name: %s\n", mutex.Name())

	// Output:
	// Lock value length: 24
	// Lock has expiry: true
	// Mutex name: lock:my-lock
}

// Example_healthCheck 演示工厂健康检查。
func Example_healthCheck() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		log.Fatal(err)
	}
	defer factory.Close()

	ctx := context.Background()

	// 健康检查
	if err := factory.Health(ctx); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed")
	}

	// 关闭后再检查
	factory.Close()
	if err := factory.Health(ctx); err != nil {
		fmt.Printf("After close: %v\n", err)
	}

	// Output:
	// Health check passed
	// After close: xdlock: factory is closed
}
