package xsemaphore_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/distributed/xsemaphore"
)

// Example_basic 演示分布式信号量的基本用法。
func Example_basic() {
	// 使用 miniredis 模拟 Redis（实际使用时换成真实 Redis）
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	// 创建信号量
	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 获取许可
	permit, err := sem.TryAcquire(ctx, "inference-task",
		xsemaphore.WithCapacity(100),      // 全局最多 100 个并发
		xsemaphore.WithTTL(5*time.Minute), // 许可 5 分钟后过期
	)
	if err != nil {
		log.Fatal(err)
	}
	if permit == nil {
		fmt.Println("System busy, try again later")
		return
	}
	defer func() {
		if err := permit.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}()

	fmt.Println("Permit acquired:", permit.ID() != "")
	fmt.Println("Resource:", permit.Resource())

	// Output:
	// Permit acquired: true
	// Resource: inference-task
}

// Example_tenantQuota 演示租户配额限制。
func Example_tenantQuota() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 模拟租户 A 获取 2 个许可
	permits := make([]xsemaphore.Permit, 0, 3)

	for i := 0; i < 2; i++ {
		permit, err := sem.TryAcquire(ctx, "shared-resource",
			xsemaphore.WithCapacity(100),  // 全局容量 100
			xsemaphore.WithTenantQuota(3), // 每租户最多 3 个
			xsemaphore.WithTenantID("tenant-A"),
		)
		if err != nil {
			log.Fatal(err)
		}
		if permit != nil {
			permits = append(permits, permit)
		}
	}

	fmt.Printf("Tenant A acquired %d permits\n", len(permits))

	// 释放许可
	for _, p := range permits {
		if err := p.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}

	// Output:
	// Tenant A acquired 2 permits
}

// Example_capacityLimit 演示容量满时的行为。
func Example_capacityLimit() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 获取 3 个许可（容量为 3）
	permits := make([]xsemaphore.Permit, 0, 4)
	for i := 0; i < 4; i++ {
		permit, err := sem.TryAcquire(ctx, "limited-resource",
			xsemaphore.WithCapacity(3),
			xsemaphore.WithTTL(time.Minute),
		)
		if err != nil {
			log.Fatal(err)
		}
		if permit != nil {
			permits = append(permits, permit)
		}
	}

	fmt.Printf("Acquired %d permits (capacity is 3)\n", len(permits))

	// 释放一个，再尝试获取
	if len(permits) > 0 {
		if err := permits[0].Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
		permits = permits[1:]

		permit, err := sem.TryAcquire(ctx, "limited-resource",
			xsemaphore.WithCapacity(3),
		)
		if err != nil {
			log.Fatal(err)
		}
		if permit != nil {
			fmt.Println("After release, acquired new permit")
			permits = append(permits, permit)
		}
	}

	// 清理
	for _, p := range permits {
		if err := p.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}

	// Output:
	// Acquired 3 permits (capacity is 3)
	// After release, acquired new permit
}

// Example_blockingAcquire 演示阻塞式获取许可。
func Example_blockingAcquire() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用 Acquire 阻塞获取（会重试直到成功或超时）
	permit, err := sem.Acquire(ctx, "blocking-resource",
		xsemaphore.WithCapacity(10),
		xsemaphore.WithMaxRetries(3),
		xsemaphore.WithRetryDelay(50*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := permit.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}()

	fmt.Println("Blocking acquire succeeded")

	// Output:
	// Blocking acquire succeeded
}

// Example_extend 演示许可续期。
func Example_extend() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 获取许可
	permit, err := sem.TryAcquire(ctx, "long-task",
		xsemaphore.WithCapacity(10),
		xsemaphore.WithTTL(time.Minute),
	)
	if err != nil {
		log.Fatal(err)
	}
	if permit == nil {
		log.Fatal("failed to acquire permit")
	}
	defer func() {
		if err := permit.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}()

	originalExpiry := permit.ExpiresAt()

	// 手动续期
	if err := permit.Extend(ctx); err != nil {
		log.Fatal(err)
	}

	newExpiry := permit.ExpiresAt()
	fmt.Printf("Permit extended: %v\n", newExpiry.After(originalExpiry))

	// Output:
	// Permit extended: true
}

// Example_query 演示查询资源状态。
func Example_query() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 先获取一些许可
	permits := make([]xsemaphore.Permit, 0, 3)
	for i := 0; i < 3; i++ {
		permit, err := sem.TryAcquire(ctx, "query-resource",
			xsemaphore.WithCapacity(10),
			xsemaphore.WithTenantQuota(5),
			xsemaphore.WithTenantID("tenant-X"),
		)
		if err != nil {
			log.Fatal(err)
		}
		if permit != nil {
			permits = append(permits, permit)
		}
	}

	// 查询状态
	info, err := sem.Query(ctx, "query-resource",
		xsemaphore.QueryWithCapacity(10),
		xsemaphore.QueryWithTenantQuota(5),
		xsemaphore.QueryWithTenantID("tenant-X"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Global: %d/%d used\n", info.GlobalUsed, info.GlobalCapacity)
	fmt.Printf("Tenant: %d/%d used\n", info.TenantUsed, info.TenantQuota)

	// 清理
	for _, p := range permits {
		if err := p.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}

	// Output:
	// Global: 3/10 used
	// Tenant: 3/5 used
}

// Example_healthCheck 演示健康检查。
func Example_healthCheck() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	sem, err := xsemaphore.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 健康检查
	if err := sem.Health(ctx); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed")
	}

	// 关闭后再检查
	if err := sem.Close(context.Background()); err != nil {
		// example test: log close error
		_ = err
	}
	if err := sem.Health(ctx); err != nil {
		fmt.Printf("After close: %v\n", err)
	}

	// Output:
	// Health check passed
	// After close: xsemaphore: semaphore is closed
}

// Example_fallback 演示降级策略。
func Example_fallback() {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if err := client.Close(); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	// 创建带降级的信号量
	sem, err := xsemaphore.New(client,
		xsemaphore.WithFallback(xsemaphore.FallbackLocal),
		xsemaphore.WithPodCount(3), // 假设 3 个 Pod
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sem.Close(context.Background()); err != nil {
			// example test: log close error
			_ = err
		}
	}()

	ctx := context.Background()

	// 正常获取许可
	permit, err := sem.TryAcquire(ctx, "fallback-resource",
		xsemaphore.WithCapacity(30),
	)
	if err != nil {
		log.Fatal(err)
	}
	if permit != nil {
		fmt.Println("Acquired permit with fallback enabled")
		if err := permit.Release(ctx); err != nil {
			// example test: log release error
			_ = err
		}
	}

	// Output:
	// Acquired permit with fallback enabled
}
