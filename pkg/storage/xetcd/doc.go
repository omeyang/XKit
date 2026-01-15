// Package xetcd 提供 etcd 客户端封装。
//
// xetcd 是 xkit 存储模块的一部分，提供：
//   - 简化的 KV 操作 (Get/Put/Delete/List)
//   - Watch 功能，监听键值变化
//   - 与 xdlock 分布式锁的集成
//
// # 快速开始
//
//	config := &xetcd.Config{
//	    Endpoints: []string{"localhost:2379"},
//	}
//	client, err := xetcd.NewClient(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// KV 操作
//	err = client.Put(ctx, "key", []byte("value"))
//	value, err := client.Get(ctx, "key")
//
// # Watch 示例
//
//	events, _ := client.Watch(ctx, "/prefix/", xetcd.WithPrefix())
//	for event := range events {
//	    fmt.Printf("%s: %s\n", event.Key, event.Value)
//	}
//
// # 与 xdlock 集成
//
// xetcd 提供的 Config 类型与 xdlock.EtcdConfig 兼容，可以复用配置：
//
//	config := &xetcd.Config{Endpoints: []string{"localhost:2379"}}
//
//	// 用于 KV 操作
//	kvClient, _ := xetcd.NewClient(config)
//
//	// 用于分布式锁（xdlock 内部引用 xetcd）
//	lockFactory, _, _ := xdlock.NewEtcdFactoryFromConfig(config, nil)
package xetcd
