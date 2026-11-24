package xcache_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/storage/xcache"
)

func ExampleNewRedis() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	// 创建 Redis 客户端
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 创建 Redis 缓存
	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	// 基础操作：直接使用底层客户端
	err = cache.Client().Set(ctx, "user:1", `{"name":"Alice"}`, time.Hour).Err()
	if err != nil {
		log.Fatal(err)
	}

	value, err := cache.Client().Get(ctx, "user:1").Result()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
	// Output: {"name":"Alice"}
}

func ExampleNewMemory() {
	// 创建内存缓存
	cache, err := xcache.NewMemory(
		xcache.WithMemoryMaxCost(10 * 1024 * 1024), // 10MB
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	// 基础操作：直接使用底层客户端
	cache.Client().SetWithTTL("key1", []byte("value1"), 6, time.Minute)

	// 等待写入完成（ristretto 异步写入）
	cache.Wait()

	// 获取值
	value, found := cache.Client().Get("key1")
	if !found {
		log.Fatal("key not found")
	}

	fmt.Println(string(value))
	// Output: value1
}

func ExampleNewLoader() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	// 创建 Loader - 这是 xcache 的核心增值功能
	loader, err := xcache.NewLoader(cache,
		xcache.WithSingleflight(true), // 防止缓存击穿
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// 使用 Load 方法 - 自动处理缓存未命中
	value, err := loader.Load(ctx, "user:1", func(ctx context.Context) ([]byte, error) {
		// 这里通常是数据库查询或 API 调用
		return []byte(`{"id":1,"name":"Alice"}`), nil
	}, time.Hour)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(value))
	// Output: {"id":1,"name":"Alice"}
}

func Example_distributedLock() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	// 获取分布式锁 - xcache 增值功能
	unlock, err := cache.Lock(ctx, "resource:1", 10*time.Second)
	if err != nil {
		log.Fatal(err)
	}

	// 执行需要锁保护的操作
	fmt.Println("Lock acquired")

	// 释放锁
	if err := unlock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Lock released")
	// Output:
	// Lock acquired
	// Lock released
}

func Example_hashOperations() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	// Hash 操作：直接使用底层客户端 - 适用于租户隔离场景
	tenantKey := "tenant:123"

	// 设置多个字段
	_ = cache.Client().HSet(ctx, tenantKey, "config_a", "value_a").Err()
	_ = cache.Client().HSet(ctx, tenantKey, "config_b", "value_b").Err()

	// 获取单个字段
	valueA, err := cache.Client().HGet(ctx, tenantKey, "config_a").Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("config_a:", valueA)

	// 获取所有字段
	all, err := cache.Client().HGetAll(ctx, tenantKey).Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("total fields:", len(all))
	// Output:
	// config_a: value_a
	// total fields: 2
}

func Example_loaderWithTTLJitter() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	// 创建带 TTL 抖动的 Loader，防止缓存雪崩
	loader, err := xcache.NewLoader(cache,
		xcache.WithTTLJitter(0.1), // 10% 抖动：1h TTL → 约 57-63 分钟
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	value, err := loader.Load(ctx, "config:app", func(ctx context.Context) ([]byte, error) {
		return []byte(`{"feature":"enabled"}`), nil
	}, time.Hour)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(value))
	// Output: {"feature":"enabled"}
}

func Example_loaderWithDistributedLock() {
	// 使用 miniredis 进行测试
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := xcache.NewRedis(client)
	if err != nil {
		log.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	// 创建带分布式锁的 Loader
	// 适用于多实例部署场景，防止缓存击穿
	loader, err := xcache.NewLoader(cache,
		xcache.WithSingleflight(true),
		xcache.WithDistributedLock(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 模拟从数据库加载数据
	value, err := loader.Load(ctx, "config:global", func(ctx context.Context) ([]byte, error) {
		return []byte(`{"version":"1.0"}`), nil
	}, time.Hour)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(value))
	// Output: {"version":"1.0"}
}
