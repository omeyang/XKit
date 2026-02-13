# 07 — 分布式模块审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 2-4** — xdlock → xetcd；xcron → xdlock + xretry；xsemaphore → xtenant + xlog + xid（最高依赖）

## 审查范围

```
pkg/distributed/
├── xdlock/      # 分布式锁（etcd Session + Redis Redlock）
├── xcron/       # 分布式定时任务（robfig/cron + 分布式锁）
└── xsemaphore/  # 分布式信号量（Redis + 本地降级），含 xsemaphoremock/ 子包
```

## 推荐 Skills

```
/code-reviewer    — 综合质量扫描
/redis-go         — Redis 分布式锁/Lua 脚本（xdlock, xsemaphore）
/etcd-go          — etcd Session/Lease/锁（xdlock）
/resilience-go    — 降级策略、重试（xsemaphore fallback）
/otel-go          — 分布式系统 trace/metrics
/design-patterns  — 状态机、策略模式、模板方法
/golang-patterns  — sync 原语、goroutine 生命周期
/go-test          — 并发测试、Lua 脚本测试、mock 策略
```

---

## 模块内部依赖链

```
xdlock → xetcd（06-存储）
xcron → xdlock, xretry（05-韧性）, xmetrics（04-可观测性）, xlog（04-可观测性）
xsemaphore → xid（01-工具包）, xtenant（02-上下文）, xlog（04-可观测性）
```

审查顺序建议：xdlock → xcron → xsemaphore

---

## 模块职责与审查要点

### xdlock — 分布式锁

**职责**：统一分布式锁接口，支持 etcd（Session 自动续期）和 Redis（Redlock 算法）两种后端。

**重点审查**：

#### 锁正确性（分布式锁最核心的属性）
- [ ] **互斥性**：同一时刻是否只有一个持有者？网络分区时 etcd/Redis 各自的互斥保证？
- [ ] **无死锁**：锁持有者崩溃后锁是否自动释放？etcd 通过 Lease TTL，Redis 通过 key TTL？
- [ ] **所有权验证**：释放锁时是否验证只有持有者能释放（Redis 用 value 比对，etcd 用 revision）？
- [ ] **锁重入**：是否支持？如果不支持，同一 goroutine 重复加锁是否会死锁？

#### etcd 后端
- [ ] **Session 管理**：Session 的 TTL 与业务操作时长是否匹配？TTL 过短导致锁意外释放？
- [ ] **KeepAlive 失败**：网络抖动导致 KeepAlive 失败时，是否有通知机制告知锁持有者？
- [ ] **选举 vs 锁**：etcd 的 election 和 lock 是否明确区分使用场景？

#### Redis 后端（Redlock）
- [ ] **Redlock 安全性**：是否严格实现 Redlock 算法？多数节点获取 + 时钟偏移补偿？
- [ ] **时钟跳跃**：Redlock 依赖时钟，NTP 跳跃是否有防护？
- [ ] **重试策略**：获取锁失败时的重试逻辑？退避策略？

#### API 设计
- [ ] **统一接口**：etcd 和 Redis 后端是否实现相同接口？切换后端是否透明？
- [ ] **工厂方法**：`NewEtcdFactory` / `NewRedisFactory` 的配置项是否对等？
- [ ] **超时控制**：TryLock（非阻塞）和 Lock（阻塞带超时）是否都支持？

### xcron — 分布式定时任务

**职责**：基于 robfig/cron + 分布式锁实现单副本执行的定时任务。

**重点审查**：

#### 分布式执行
- [ ] **单副本保证**：多个 Pod 同时触发 cron 时，是否只有一个执行？Locker 的竞争窗口有多大？
- [ ] **锁释放时机**：任务执行完成后锁立即释放，还是持有到下次触发？长任务场景？
- [ ] **任务重叠**：前一次执行未完成，下一次触发到来时的行为？跳过？排队？

#### Locker 策略
- [ ] **NoopLocker**：单副本场景，是否真的无锁直接执行？误用于多副本场景的后果？
- [ ] **RedisLocker**：Redis SET NX 的 TTL 是否与 cron 间隔匹配？TTL 过长导致后续触发被跳过？
- [ ] **K8sLocker**：Kubernetes Lease 资源的续期间隔与 Lease Duration 的关系？节点故障转移时间？

#### 错误处理
- [ ] **任务 panic**：cron 任务中 panic 是否 recover？是否影响调度器本身？
- [ ] **重试集成**：任务失败时是否支持自动重试（集成 xretry）？重试不应与下次触发冲突
- [ ] **错误通知**：连续失败是否有告警机制？

#### 可观测性
- [ ] **执行日志**：每次执行是否有日志（开始、结束、耗时、结果）？
- [ ] **metrics**：执行次数、耗时、成功/失败率是否有 metrics？
- [ ] **trace**：每次 cron 执行是否创建独立的 trace span？

### xsemaphore — 分布式信号量（高复杂度，重点审查）

**职责**：Redis 基分布式信号量，支持多租户、本地降级、Lua 脚本、ID 生成注入。

**重点审查**：

#### 核心正确性
- [ ] **信号量语义**：acquire/release/query 是否符合经典信号量语义？
- [ ] **Permit 生命周期**：acquire → hold → release/expire 的状态转换是否完整？
- [ ] **容量控制**：并发 acquire 是否正确限制在 capacity 内？边界条件（capacity=0, 1）？
- [ ] **TTL 过期**：过期 permit 是否自动清理？清理时机（主动清理 vs 惰性清理）？
- [ ] **Extend 正确性**：续期是否正确更新 TTL？已释放的 permit 续期是否返回错误？

#### Lua 脚本安全
- [ ] **原子性**：acquire/release/query 的 Lua 脚本是否保证原子执行？
- [ ] **KEYS 使用**：是否使用 hash tag `{}`确保相关 key 在同一 slot？集群模式兼容？
- [ ] **只读脚本**：query.lua 是否为纯只读（ZCOUNT 而非 ZREMRANGEBYSCORE）？
- [ ] **脚本参数化**：是否通过 KEYS/ARGV 传参（不拼接字符串）？防注入？
- [ ] **错误处理**：Lua 脚本执行失败的错误是否正确传播到 Go 层？

#### 本地降级（FallbackLocal）
- [ ] **降级触发**：什么条件触发降级（Redis 不可用？超时？）？触发条件是否可配置？
- [ ] **本地信号量正确性**：`localSemaphore` 的并发控制是否正确？是否使用 channel / sync 原语？
- [ ] **回切时机**：Redis 恢复后是否自动回切？回切时本地持有的 permit 如何处理？
- [ ] **goroutine 安全**：`ensureLocalSemaphore` 与 `Close` 的竞态条件？`localMu` + `closed` flag？
- [ ] **回调限流**：`safeOnFallback` 的 10 秒最小回调间隔是否合理？

#### 多租户与资源隔离
- [ ] **资源名校验**：是否禁止 `{}:` 和空白字符（避免 Redis key 冲突）？
- [ ] **租户隔离**：不同租户的 permit 是否存储在不同 key 下？
- [ ] **keyPrefix**：用户可配置的 key 前缀是否经过校验？

#### ID 生成注入
- [ ] **IDGeneratorFunc**：接口设计是否合理？默认使用 `xid.NewStringWithRetry`？
- [ ] **WithIDGenerator**：自定义 ID 生成器注入是否生效？测试覆盖？
- [ ] **ID 唯一性**：生成的 permit ID 在高并发下是否保证唯一？

#### 可观测性
- [ ] **Trace span**：TryAcquire/Acquire/Query/Release/Extend 是否都有 span？
- [ ] **Metrics**：操作计数（counter）、延迟（histogram）、活跃 permit 数（gauge）？
- [ ] **属性一致性**：trace 和 metrics 使用的属性键是否统一（`attrSemType` 等）？

#### 资源管理
- [ ] **Close 顺序**：停止 goroutine → 清理本地信号量 → 释放 Redis 连接？
- [ ] **orphan permit 清理**：`cleanupAllExpired` 是否有 race（不应删除空 map entry）？
- [ ] **重复 Close**：多次调用 Close 是否安全（幂等）？

#### Options 校验
- [ ] **fail-fast**：`validate()` 在 `New()`/`TryAcquire`/`Acquire`/`Query` 中是否都调用？
- [ ] **无效值拒绝**：负数 capacity、空 resource name、非法 keyPrefix 是否立即返回错误？
- [ ] **错误变量**：`ErrInvalidMaxRetries`、`ErrInvalidRetryDelay` 等是否完整？

---

## 跨包一致性检查

- [ ] xdlock 的 Redis 锁与 xsemaphore 的 Redis 使用是否一致（连接管理、key 命名规范）？
- [ ] xcron 的 Locker 接口与 xdlock 的锁接口是否可互通？
- [ ] xsemaphore 的 IDGeneratorFunc 与 xid 的 API 是否匹配？
- [ ] 三个包的 metrics 是否使用 xmetrics 接口？指标命名是否遵循 `xkit.distributed.*` 前缀？
- [ ] 所有 Lua 脚本是否统一放在包内（embedded files）？加载方式是否一致？
- [ ] 分布式包的超时控制是否都通过 context deadline 传递？
