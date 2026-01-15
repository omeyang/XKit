package xetcd_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/omeyang/xkit/pkg/storage/xetcd"
)

// ExampleNewClient 演示如何创建 etcd 客户端。
func ExampleNewClient() {
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("Client created successfully")
	// Output: Client created successfully
}

// ExampleNewClient_withHealthCheck 演示带健康检查的客户端创建。
func ExampleNewClient_withHealthCheck() {
	config := &xetcd.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	}

	// 创建客户端并执行健康检查
	_, err := xetcd.NewClient(config,
		xetcd.WithHealthCheck(true, 3*time.Second),
	)
	if err != nil {
		// 健康检查失败或连接失败
		fmt.Println("Failed to create client:", err)
		return
	}

	fmt.Println("Client with health check created")
}

// ExampleDefaultConfig 演示使用默认配置。
func ExampleDefaultConfig() {
	config := xetcd.DefaultConfig()
	config.Endpoints = []string{"localhost:2379"}

	fmt.Printf("DialTimeout: %v\n", config.DialTimeout)
	fmt.Printf("DialKeepAliveTime: %v\n", config.DialKeepAliveTime)
	// Output:
	// DialTimeout: 5s
	// DialKeepAliveTime: 10s
}

// ExampleClient_Get 演示获取键值。
func ExampleClient_Get() {
	// 假设客户端已创建
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// 获取键值
	value, err := client.Get(ctx, "/app/config/name")
	if err != nil {
		if xetcd.IsKeyNotFound(err) {
			fmt.Println("Key not found")
			return
		}
		log.Fatal(err)
	}

	fmt.Printf("Value: %s\n", value)
}

// ExampleClient_Put 演示设置键值。
func ExampleClient_Put() {
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// 设置键值
	err = client.Put(ctx, "/app/config/name", []byte("myapp"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Value set successfully")
}

// ExampleClient_PutWithTTL 演示设置带 TTL 的键值。
func ExampleClient_PutWithTTL() {
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// 设置 10 秒后过期的键值
	err = client.PutWithTTL(ctx, "/app/session/abc123", []byte("user-data"), 10*time.Second)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Value with TTL set successfully")
}

// ExampleClient_List 演示列出指定前缀的所有键值。
func ExampleClient_List() {
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// 列出 /app/config/ 前缀下的所有键值
	kvs, err := client.List(ctx, "/app/config/")
	if err != nil {
		log.Fatal(err)
	}

	for key, value := range kvs {
		fmt.Printf("%s = %s\n", key, value)
	}
}

// ExampleClient_Watch 演示监听键值变化。
func ExampleClient_Watch() {
	config := &xetcd.Config{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := xetcd.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 监听 /app/config/ 前缀下的所有变化
	events, err := client.Watch(ctx, "/app/config/", xetcd.WithPrefix())
	if err != nil {
		log.Fatal(err)
	}

	// 处理事件
	for event := range events {
		switch event.Type {
		case xetcd.EventPut:
			fmt.Printf("PUT: %s = %s\n", event.Key, event.Value)
		case xetcd.EventDelete:
			fmt.Printf("DELETE: %s\n", event.Key)
		}
	}
}

// ExampleIsKeyNotFound 演示错误处理。
func ExampleIsKeyNotFound() {
	err := xetcd.ErrKeyNotFound

	if xetcd.IsKeyNotFound(err) {
		fmt.Println("Key does not exist")
	}
	// Output: Key does not exist
}

// ExampleIsClientClosed 演示客户端关闭检查。
func ExampleIsClientClosed() {
	err := xetcd.ErrClientClosed

	if xetcd.IsClientClosed(err) {
		fmt.Println("Client has been closed")
	}
	// Output: Client has been closed
}
