// Package storage 提供数据存储相关的子包。
//
// 子包列表：
//   - xcache: 缓存抽象层，支持 Redis 和内存缓存
//   - xmongo: MongoDB 客户端封装
//   - xclickhouse: ClickHouse 客户端封装
//
// 设计原则：
//   - 提供统一的接口抽象，支持多种存储后端
//   - 内置可观测性（指标、追踪）
//   - 支持连接池和重试策略
package storage
