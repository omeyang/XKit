// Package xetcd 提供 etcd 客户端封装。
//
// xetcd 是 xkit 存储模块的一部分，提供：
//   - 简化的 KV 操作 (Get/Put/Delete/List)
//   - Watch 功能，监听键值变化
//   - 与 xdlock 分布式锁的集成
//
// # 与 xdlock 集成
//
// xetcd 提供的 Config 类型与 xdlock.EtcdConfig 兼容，可以复用配置。
package xetcd
