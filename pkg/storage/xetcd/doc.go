// Package xetcd 提供 etcd 客户端封装。
//
// xetcd 是 xkit 存储模块的一部分，提供：
//   - 简化的 KV 操作 (Get/Put/Delete/List/Exists/Count)
//   - PutWithTTL 带租约的键值写入
//   - Watch 功能，监听键值变化
//   - WatchWithRetry 带自动重连和指数退避（含随机抖动）的 Watch
//   - 与 xdlock 分布式锁的集成
//
// # 错误处理
//
// 所有接受 context.Context 的公开方法（Get/Put/Delete/Watch 等）在 ctx 为 nil 时
// 返回 ErrNilContext，避免 nil ctx 传递到 etcd 客户端导致 panic。
// Close 方法例外：ctx 参数当前仅用于未来扩展（D-02 决策），nil 时不返回错误。
//
// # 设计边界
//
// 设计决策: xetcd 定位为简化的 KV + Watch 封装，不提供以下高级功能：
//   - 事务（Txn/CAS/PutIfAbsent）：通过 RawClient() 使用原生 etcd 事务 API
//   - 租约续约（KeepAlive）：通过 RawClient() 使用原生租约 API
//
// 这些功能的使用场景（服务注册、分布式选主等）需要更复杂的生命周期管理，
// 由调用方根据具体需求直接操作 RawClient() 更为灵活。
//
// 设计决策: xetcd 不内建连接重试和周期性健康探测。
// 初始化重试和持续健康检查属于上层框架的职责（如服务启动编排、健康检查端点），
// xetcd 作为基础客户端封装不应假设调用方的重试策略。
// WithHealthCheck 提供一次性创建阶段检查，满足 fail-fast 需求。
//
// 设计决策: xetcd 不内建可观测性（Trace/Metrics）。
// etcd 官方客户端已通过 gRPC interceptor 提供基础的 RPC 级别追踪和指标，
// 调用方可通过 gRPC DialOption 注入自定义 interceptor。
// 若未来需要统一可观测性，可通过 WithTracer/WithMeter Option 扩展，
// 当前阶段避免引入 xmetrics 依赖以保持包的轻量性。
//
// # 与 xdlock 集成
//
// xetcd 提供的 Config 类型与 xdlock.EtcdConfig 兼容，可以复用配置。
package xetcd
