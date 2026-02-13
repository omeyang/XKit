# 05 — 韧性模块审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 1-3** — xretry 无依赖；xbreaker → xretry；xlimit → xconf + xtenant + xlog + xmetrics

## 审查范围

```
pkg/resilience/
├── xretry/    # 重试策略，基于 avast/retry-go/v5
├── xbreaker/  # 熔断器，基于 sony/gobreaker/v2
└── xlimit/    # 限流器

internal/
└── （相关内部实现）
```

## 推荐 Skills

```
/code-reviewer    — 综合质量扫描
/resilience-go    — 熔断/重试/限流最佳实践（核心）
/go-style         — 接口设计、错误分类
/golang-patterns  — 策略模式、装饰器模式、组合模式
/design-patterns  — 熔断器状态机、限流算法
/go-test          — 并发测试、时间相关测试
```

---

## 模块内部依赖链

```
xretry（独立）
xbreaker → xretry（可选组合）
xlimit → xconf, xtenant, xlog, xmetrics（重度依赖）
```

审查顺序建议：xretry → xbreaker → xlimit

---

## 模块职责与审查要点

### xretry — 重试策略

**职责**：RetryPolicy/BackoffPolicy 接口，支持 Retryer 和 retry-go 两种风格，错误分类。

**重点审查**：

#### 接口设计
- [ ] **RetryPolicy 接口**：是否足够抽象以支持不同重试策略（固定/指数退避/自定义）？
- [ ] **BackoffPolicy 接口**：退避时间计算是否包含 jitter？无 jitter 会导致"惊群效应"
- [ ] **双风格共存**：Retryer 和 retry-go 两种风格是否有必要同时存在？是否增加使用方认知负担？
- [ ] **类型别名**：暴露 retry-go 底层类型的类型别名是否合适？版本升级时是否存在破坏风险？

#### 错误分类
- [ ] **Permanent/Temporary/Unrecoverable**：三种错误分类的语义是否清晰？边界是否明确？
- [ ] **错误判断**：`errors.Is` / `errors.As` 是否正确用于判断错误类型？自定义 error 是否实现 `Unwrap()`？
- [ ] **context 取消**：`context.Canceled` 和 `context.DeadlineExceeded` 是否被视为不可重试？

#### 行为正确性
- [ ] **最大重试次数**：`maxRetries` 的含义是否明确（总尝试次数 vs 重试次数）？文档是否与实现一致？
- [ ] **退避溢出**：指数退避在极端重试次数下是否存在整数溢出？是否有最大退避时间上限？
- [ ] **context 尊重**：退避等待期间是否检查 context 取消？不应在 context 已取消后继续等待

### xbreaker — 熔断器

**职责**：TripPolicy 熔断策略，状态机（Closed/Open/HalfOpen），与 xretry 可组合。

**重点审查**：

#### 状态机
- [ ] **状态转换正确性**：Closed → Open → HalfOpen → Closed 的转换条件是否正确？
- [ ] **HalfOpen 探测**：HalfOpen 状态下允许多少请求通过？探测成功/失败的判定逻辑？
- [ ] **并发状态转换**：多个 goroutine 同时触发状态转换时是否安全？是否存在 race condition？
- [ ] **状态可观测**：当前状态是否可以通过 API 查询？状态变化是否有回调或日志？

#### TripPolicy
- [ ] **策略设计**：是否支持多种触发策略（连续失败次数、失败率、慢请求率）？
- [ ] **滑动窗口**：计数器/失败率的统计窗口是否可配置？窗口大小选择是否有指导？
- [ ] **重置逻辑**：Closed 状态下计数器是否定期重置（避免历史错误累积）？

#### 组合使用
- [ ] **BreakerRetryer**：熔断 + 重试组合时，重试是否在熔断器允许的情况下执行？
- [ ] **错误透传**：熔断器拒绝请求时返回的错误类型是否可被 xretry 识别为 Permanent（不应重试）？
- [ ] **指标集成**：熔断器状态变化、拒绝计数是否暴露 metrics？

### xlimit — 限流器

**职责**：限流策略，支持多种算法和多租户场景。

**重点审查**：

#### 算法正确性
- [ ] **限流算法**：使用哪种算法（令牌桶/滑动窗口/漏桶/固定窗口）？选择理由是否充分？
- [ ] **精度**：滑动窗口在窗口边界处是否有精度损失？对突发流量的处理是否合理？
- [ ] **分布式限流**：是否支持跨节点限流（基于 Redis）？本地限流与分布式限流的一致性？

#### 多租户支持
- [ ] **租户隔离**：不同租户是否有独立的限流计数器？一个租户的流量突增不影响其他租户？
- [ ] **动态配置**：限流阈值是否支持按租户动态调整？调整后是否立即生效？
- [ ] **与 xtenant 集成**：租户信息从 context 提取是否与 xtenant 的 API 一致？

#### 性能与并发
- [ ] **热路径性能**：限流检查在每个请求路径上，延迟开销是多少？是否有 benchmark？
- [ ] **锁竞争**：高并发场景下限流器内部锁是否成为瓶颈？
- [ ] **内存**：大量租户/key 场景下内存占用如何增长？是否有过期清理机制？

#### 降级策略
- [ ] **限流后行为**：超限后返回什么（立即拒绝？排队等待？）是否可配置？
- [ ] **错误信息**：限流拒绝返回的错误是否包含重试建议（如 Retry-After header）？
- [ ] **限流器不可用**：Redis 限流器连接失败时是否有降级策略（如切换到本地限流）？

---

## 跨包一致性检查

- [ ] xretry 的错误分类（Permanent/Temporary）是否与 xbreaker 的错误判定一致？
- [ ] xlimit 的 metrics 是否使用 xmetrics 接口（而非直接依赖 OTel SDK）？
- [ ] xlimit 的租户提取是否使用 xctx/xtenant 的标准 API？
- [ ] xbreaker + xretry 的组合模式是否有标准示例和文档说明？
- [ ] 三个包的 Options 模式（`WithXxx`）命名风格是否一致？
