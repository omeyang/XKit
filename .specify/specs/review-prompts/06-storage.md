# 06 — 存储模块审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 1-3** — xcache/xetcd 无内部依赖；xmongo/xclickhouse → xmetrics

## 审查范围

```
pkg/storage/
├── xcache/       # 缓存统一封装（Redis + 内存/ristretto）
├── xetcd/        # etcd 客户端封装
├── xmongo/       # MongoDB 包装器
└── xclickhouse/  # ClickHouse 包装器
```

## 推荐 Skills

```
/code-reviewer    — 综合质量扫描
/redis-go         — Redis 缓存模式、连接管理、Lua 脚本（xcache）
/etcd-go          — etcd KV/Watch/Lease 最佳实践（xetcd）
/mongodb-go       — MongoDB 查询/聚合/索引/分页（xmongo）
/clickhouse-go    — ClickHouse 表引擎/批量插入/分页（xclickhouse）
/go-style         — 接口设计、包装器模式
/golang-patterns  — Cache-Aside 模式、singleflight
/go-test          — 集成测试、mock 策略
```

---

## 设计原则

所有存储包遵循统一设计：

1. **工厂方法创建**：`New()` / `NewRedis()` / `NewMemory()` 等
2. **底层客户端暴露**：`Client()` / `Conn()` 方法暴露原始客户端
3. **增值功能叠加**：健康检查、统计、慢查询检测、分页等
4. **Client() 旁路**：通过 Client() 直接操作不经过增值层（设计意图，需文档说明）

审查时需验证：每个包是否一致遵循上述模式，以及旁路的风险是否有文档警告。

---

## 模块职责与审查要点

### xcache — 缓存统一封装

**职责**：Redis/Memory 双后端缓存，Loader 提供 Cache-Aside 模式（singleflight + 分布式锁）。

**重点审查**：

#### 缓存一致性
- [ ] **Cache-Aside 正确性**：查缓存 → 未命中 → 加载数据 → 写缓存，每一步的错误处理？
- [ ] **singleflight**：并发请求同一 key 时是否正确合并？singleflight panic 是否影响其他等待者？
- [ ] **分布式锁（WithExternalLock）**：锁超时后缓存是否可能被脏写？锁续期机制？
- [ ] **缓存穿透**：空结果是否缓存（null object pattern）？缓存时间是否短于正常值？
- [ ] **缓存雪崩**：TTL 是否有随机化（jitter）？热 key 过期时是否有预热机制？

#### 序列化
- [ ] **值序列化**：使用什么序列化方式（JSON/MessagePack/gob）？是否可配置？
- [ ] **类型安全**：Get 返回 `interface{}` 还是泛型 `T`？是否存在类型断言失败风险？
- [ ] **大值处理**：超大值存入 Redis 时是否有限制/告警？

#### 双后端一致性
- [ ] **接口统一**：Redis 和 Memory 后端是否实现完全相同的接口？是否有行为差异？
- [ ] **Memory 后端线程安全**：内存缓存（ristretto）并发读写是否安全？
- [ ] **切换透明**：从 Memory 切换到 Redis（或反过来）是否需要修改业务代码？

#### 资源管理
- [ ] **连接池**：Redis 连接池大小是否可配置？连接泄漏如何检测？
- [ ] **关闭顺序**：`Close()` 时是否先停止 Loader 再关闭连接？

### xetcd — etcd 客户端封装

**职责**：简化的 KV 操作、Watch 功能。

**重点审查**：
- [ ] **Watch 稳定性**：网络断开后 Watch 是否自动重连？是否有事件丢失风险？
- [ ] **Lease 管理**：Lease 是否自动续期（KeepAlive）？续期失败时 key 是否正确过期？
- [ ] **事务支持**：是否暴露 etcd 事务（Txn）？如果不暴露，复杂操作如何保证原子性？
- [ ] **连接管理**：连接失败的重试策略？健康检查频率？
- [ ] **Client() 暴露**：调用方直接使用 etcd client 是否会绕过日志/metrics 收集？

### xmongo — MongoDB 包装器

**职责**：分页查询、批量写入、慢查询检测。

**重点审查**：

#### 查询
- [ ] **分页实现**：是否同时支持 offset 和 cursor 分页？cursor 分页是否正确使用索引？
- [ ] **投影**：是否支持字段投影（只返回需要的字段）？减少数据传输？
- [ ] **慢查询检测**：阈值是否可配置？检测结果是日志还是 metrics（或两者）？
- [ ] **查询超时**：是否给每个查询设置 context timeout？默认超时是否合理？

#### 写入
- [ ] **批量写入**：batch size 是否可配置？单批过大是否有拆分机制？
- [ ] **写关注（Write Concern）**：默认写关注级别是否合适？是否可配置？
- [ ] **幂等写入**：是否支持 upsert？批量写入部分失败时的行为？

#### 连接与资源
- [ ] **连接池**：MongoDB 连接池配置是否暴露？默认值是否合理？
- [ ] **index 提示**：是否提供 index hint 能力？避免查询计划选择错误？

### xclickhouse — ClickHouse 包装器

**职责**：分页查询、批量插入。

**重点审查**：
- [ ] **批量插入**：是否使用 ClickHouse 原生批量写入协议（不是逐行 INSERT）？
- [ ] **分页查询**：ClickHouse 不适合 offset 分页（全表扫描），是否引导使用者用 WHERE 条件分页？
- [ ] **类型映射**：Go 类型与 ClickHouse 类型（DateTime64, LowCardinality 等）的映射是否正确？
- [ ] **异步插入**：是否支持 ClickHouse 的异步插入（`async_insert`）以提升吞吐？
- [ ] **查询超时**：大查询是否有超时控制？避免阻塞 ClickHouse 集群？
- [ ] **连接管理**：ClickHouse 连接是否支持多副本负载均衡？

---

## 跨包一致性检查

- [ ] 四个存储包的工厂方法签名是否一致（`New(opts ...Option)`）？
- [ ] `Client()` / `Conn()` 的命名是否统一（不要一个用 Client 一个用 Conn）？
- [ ] 健康检查接口是否统一（如 `Ping(ctx) error`）？
- [ ] 慢操作检测（xcache, xmongo）的阈值配置方式是否一致？
- [ ] metrics 收集是否都通过 xmetrics 接口？指标命名前缀是否统一（`xkit.storage.*`）？
- [ ] 所有存储包的 `Close()` 方法行为是否一致（等待操作完成 → 释放连接）？
- [ ] xcache 的 WithExternalLock 与 xdlock（07-distributed）的集成是否正确？
