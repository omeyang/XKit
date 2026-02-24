package xauth_test

import (
	"context"
	"fmt"
	"log"

	"github.com/omeyang/xkit/pkg/business/xauth"
)

func ExampleNewClient() {
	// 创建客户端
	client, err := xauth.NewClient(&xauth.Config{
		Host: "https://auth.example.com",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close(context.Background())

	// 使用客户端
	_ = client
	fmt.Println("client created")
	// Output: client created
}

func ExampleClient_GetToken() {
	client, err := xauth.NewClient(&xauth.Config{
		Host: "https://auth.example.com",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close(context.Background())

	ctx := context.Background()

	// 获取 Token
	token, err := client.GetToken(ctx, "tenant-123")
	if err != nil {
		// 处理错误
		log.Printf("get token failed: %v", err)
		return
	}

	// 使用 Token
	_ = token
}

func ExampleClient_GetPlatformID() {
	client, err := xauth.NewClient(&xauth.Config{
		Host: "https://auth.example.com",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close(context.Background())

	ctx := context.Background()

	// 获取平台 ID
	platformID, err := client.GetPlatformID(ctx, "tenant-123")
	if err != nil {
		log.Printf("get platform id failed: %v", err)
		return
	}

	// 使用平台 ID
	_ = platformID
}

func ExampleWithCache() {
	// 使用 Redis 缓存（需要提供 redis.UniversalClient）
	// redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	// cache, err := xauth.NewRedisCacheStore(redisClient)
	//
	// client, err := xauth.NewClient(&xauth.Config{
	//     Host: "https://auth.example.com",
	// }, xauth.WithCache(cache))

	fmt.Println("cache configured")
	// Output: cache configured
}

func Example_withOptions() {
	// 使用多个配置选项
	client, err := xauth.NewClient(&xauth.Config{
		Host: "https://auth.example.com",
	},
		xauth.WithLocalCache(true),
		xauth.WithLocalCacheMaxSize(2000),
		xauth.WithSingleflight(true),
		xauth.WithBackgroundRefresh(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close(context.Background())

	fmt.Println("client with options created")
	// Output: client with options created
}

func ExampleConfig_Validate() {
	cfg := &xauth.Config{
		Host: "https://auth.example.com",
	}

	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("config is valid")
	// Output: config is valid
}

func ExampleIsRetryable() {
	// 检查错误是否可重试
	err := xauth.ErrServerError
	if xauth.IsRetryable(err) {
		fmt.Println("error is retryable")
	}
	// Output: error is retryable
}

func ExampleNewRedisCacheStore() {
	// 创建 Redis 缓存存储
	// redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	// cache, err := xauth.NewRedisCacheStore(redisClient,
	//     xauth.WithKeyPrefix("myapp:xauth:"),
	// )
	//
	// client, err := xauth.NewClient(&xauth.Config{
	//     Host: "https://auth.example.com",
	// }, xauth.WithCache(cache))

	fmt.Println("redis cache configured")
	// Output: redis cache configured
}
