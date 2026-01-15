// Package distributed 提供分布式协调相关的子包。
//
// 子包列表：
//   - xdlock: 分布式锁，支持 Redis、etcd、K8s 后端
//   - xcron: 分布式定时任务，支持分布式锁保证单实例执行
//
// 设计原则：
//   - 提供统一的锁接口，支持多种后端实现
//   - 支持锁续期和优雅释放
//   - 内置健康检查和指标收集
package distributed
