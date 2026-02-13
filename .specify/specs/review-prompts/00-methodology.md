# XKit 代码审查方法论

## 审查哲学

XKit 是**全新的基础工具库**，不承担历史兼容包袱。审查时应以**最优架构设计**为目标：

- **追求最佳实践**：不因业务现状妥协设计，业务库应适配工具库，而非反过来
- **零容忍设计缺陷**：API 一旦发布，修改成本极高；宁可现在多花时间审查，也不要日后背负技术债
- **无需兼容**：不考虑旧接口迁移、废弃字段保留、类型别名桥接等兼容手段
- **简洁优先**：能用标准库解决的不引入外部依赖，能用一个接口表达的不设计两个

---

## 审查流程

```
1. 阅读本文件 → 理解通用方法论和评估维度
2. 按编号顺序选择模块 prompt → 依赖关系由低到高
3. 对每个模块：阅读源码 → 运行工具 → 使用 Skills → 输出报告
4. 跨模块审查 → 检查包间依赖、接口一致性、命名统一性
```

### 模块依赖顺序

按编号从小到大审查，确保审查高层模块时已理解其依赖：

| 编号 | 模块 | 包 | 依赖 |
|------|------|-----|------|
| 01 | 工具包 | xid, xfile, xjson, xkeylock, xlru, xmac, xnet, xpool, xproc, xsys, xutil | 无内部依赖 |
| 02 | 上下文 | xctx, xenv, xplatform, xtenant | 模块内依赖（xtenant → xctx, xplatform） |
| 03 | 配置 | xconf | 无内部依赖 |
| 04 | 可观测性 | xlog, xmetrics, xtrace, xsampling, xrotate | → xctx, xfile |
| 05 | 韧性 | xretry, xbreaker, xlimit | → xconf, xtenant, xlog, xmetrics |
| 06 | 存储 | xcache, xmongo, xclickhouse, xetcd | → xmetrics |
| 07 | 分布式 | xdlock, xcron, xsemaphore | → xetcd, xretry, xid, xtenant, xlog |
| 08 | 消息队列 | xkafka, xpulsar | → xmetrics, xretry |
| 09 | 业务与生命周期 | xauth, xrun, xdbg | → xctx, xmetrics, xlru |

---

## 评估维度

审查必须覆盖以下 15 个维度。每个维度下列出关键检查项，审查时逐项确认：

### 1. 架构设计

- [ ] 包职责边界清晰，不存在"上帝包"（承担过多职责的包）
- [ ] 依赖方向正确：高层依赖低层，不存在循环依赖
- [ ] 抽象层次一致：同一包内的函数/类型处于相同抽象层次
- [ ] 接口定义在使用方，而非实现方（Go 惯用法）
- [ ] 工厂方法 `New()` 返回接口而非具体类型（除非有充分理由）
- [ ] 分层合理：`pkg/` 只暴露公开 API，实现细节在 `internal/` 或包内未导出

### 2. 能力实现

- [ ] 功能完整：spec 中定义的能力是否全部实现
- [ ] 行为正确：边界条件、零值、nil 输入、空集合等是否处理
- [ ] 语义一致：函数行为是否与文档/命名完全匹配，无隐式副作用
- [ ] 契约明确：前置条件（参数校验）和后置条件（返回值保证）是否清晰

### 3. 单一职责

- [ ] 每个包只做一件事，包名即职责描述
- [ ] 每个文件聚焦一个主题（类型定义、接口、工具函数等）
- [ ] 函数 ≤70 行，参数 ≤3 个（超过用 options struct）
- [ ] 不存在"工具类"函数混入业务包

### 4. 代码质量

- [ ] `golangci-lint` 零告警（不使用 `//nolint`）
- [ ] 错误处理：`fmt.Errorf("xxx: %w", err)` 包装上下文，不吞错误
- [ ] 不使用 `panic`（除非不可恢复错误且有充分注释）
- [ ] 命名规范：标识符英文、文档中文、缩写全大写（ID, IP, HTTP）
- [ ] 导出符号不重复包名（`xcache.CacheClient` ❌ → `xcache.Client` ✅）

### 5. 可观测性

- [ ] 关键操作有 trace span（acquire/release/query 等）
- [ ] 错误/慢操作有 metrics 记录
- [ ] 日志级别合理：Debug（调试）、Info（状态变化）、Warn（可恢复异常）、Error（需介入）
- [ ] 日志包含足够上下文（resource、tenant、operation 等）
- [ ] 指标命名遵循统一前缀（如 `xkit.xxx.*`）

### 6. 可维护性

- [ ] 代码结构可预测：同类型文件（`_test.go`, `doc.go`, `options.go`）遵循统一命名
- [ ] 配置项有合理默认值，且默认值在文档/常量中可见
- [ ] 魔法数字提取为命名常量
- [ ] 不存在 dead code（未使用的导出符号、注释掉的代码块）

### 7. 可阅读性

- [ ] 函数命名自文档化，无需注释即可理解意图
- [ ] 复杂逻辑有注释解释 **为什么**（不是解释"做了什么"）
- [ ] 控制流简洁：优先 early return，避免深层嵌套（≤3 层）
- [ ] 类型和方法按逻辑顺序排列（接口 → 实现 → 工具函数）

### 8. 可测试性

- [ ] 核心逻辑覆盖率 ≥95%，整体 ≥90%
- [ ] 表驱动测试（table-driven tests）为主
- [ ] 只 mock 外部依赖（DB、网络、MQ），不 mock 核心逻辑
- [ ] 测试用例覆盖：正常路径、错误路径、边界条件、并发场景
- [ ] Mock 放在独立子包（如 `xsemaphoremock/`），不拉低覆盖率
- [ ] 测试辅助函数（helper）有 `t.Helper()` 标记

### 9. 鲁棒性

- [ ] 所有外部输入（用户参数、环境变量、Redis 返回值）有校验
- [ ] goroutine 有明确退出机制（context cancellation / done channel）
- [ ] 资源获取有对应释放（defer close、defer unlock）
- [ ] 超时控制：所有外部调用（网络、Redis、etcd）有 context deadline
- [ ] 优雅关闭：`Close()` 方法等待所有 goroutine 退出
- [ ] 幂等性：重复 close/release 不 panic

### 10. 灵活性

- [ ] 使用 Functional Options 模式配置（`WithXxx()`）
- [ ] 策略可替换：关键行为通过接口/函数参数注入
- [ ] 不硬编码环境相关值（地址、超时、阈值等均可配置）
- [ ] 合理的扩展点：新增实现不需要修改已有代码（开闭原则）

### 11. 性能表现

- [ ] 热路径无不必要的内存分配（`sync.Pool` 复用、预分配 slice）
- [ ] 无锁竞争瓶颈：读多写少用 `RWMutex`，高并发考虑 `sync.Map` / 分片锁
- [ ] 批量操作优于逐条操作（batch insert、pipeline）
- [ ] `go test -race` 零告警
- [ ] 关键路径有 benchmark 测试

### 12. 高内聚

- [ ] 包内类型和函数围绕同一概念组织
- [ ] 不存在仅被一个外部包使用的导出类型（应内聚到使用方）
- [ ] 相关常量、错误、类型定义在同一文件（如 `errors.go`, `consts.go`）

### 13. 低耦合

- [ ] 包间通过接口交互，不直接依赖具体实现
- [ ] 不存在包 A 直接读取包 B 的内部状态（只通过方法调用）
- [ ] import 列表短小：一个包不应 import 超过 3 个同项目内的包
- [ ] 不存在 import cycle（编译器会报错，但设计上也要避免"间接循环"意图）

### 14. 代码冗余

- [ ] 不存在 copy-paste 代码（≥3 处相同逻辑应提取）
- [ ] 不存在功能重叠的函数/方法
- [ ] 不存在已被取代但未删除的旧实现
- [ ] 工具函数不重复标准库能力

### 15. 安全性

- [ ] 无命令注入、路径穿越、SQL/NoSQL 注入风险
- [ ] 敏感信息（密码、token）不出现在日志和错误信息中
- [ ] Lua 脚本（Redis）使用参数化 KEYS/ARGV，不拼接字符串
- [ ] 输入校验在入口处完成（fail-fast），不依赖调用方校验

---

## 输出规范

### 严重程度分类

| 级别 | 含义 | 示例 |
|------|------|------|
| 🔴 **严重** | 生产故障、数据丢失、安全漏洞、竞态条件 | goroutine 泄漏、资源未释放、race condition |
| 🟡 **中等** | 可用性降级、性能瓶颈、可维护性风险 | 缺少超时控制、错误信息不足、接口设计不当 |
| 🟢 **轻微** | 代码规范、命名、文档、风格 | 常量未提取、注释缺失、命名不一致 |

### 报告结构

每个模块的审查报告应包含：

```markdown
# [模块名] 审查报告

## 概要
- 审查范围：包列表
- 代码行数 / 测试覆盖率
- 总体评价（一句话）

## 发现列表

### 🔴 严重问题
#### S1: [问题标题]
- **位置**：`file.go:42`
- **问题**：具体描述
- **影响**：可能造成的后果
- **建议**：修复方案

### 🟡 中等问题
（同上格式）

### 🟢 轻微问题
（同上格式）

## 架构评估
- 职责划分是否合理
- 接口设计是否符合 Go 惯用法
- 扩展性评估

## 亮点
- 做得好的设计决策（正面反馈同样重要）
```

### 输出要求

- 用中文输出审查结果
- **不直接修改代码**，只分析问题并提出改进建议
- 每个问题必须标注**位置**（文件名:行号）和**影响**
- 建议新功能时必须回答：解决什么问题？不加会怎样？

---

## 审查工具链

### 静态分析（必选）

```bash
# golangci-lint（项目已配置，涵盖 errcheck、funlen、govet 等）
task lint

# 竞态检测
go test -race ./pkg/xxx/...

# 覆盖率
go test -coverprofile=cover.out ./pkg/xxx/...
go tool cover -func=cover.out
```

### 依赖分析

```bash
# 包级依赖
go list -f '{{.ImportPath}} → {{.Imports}}' ./pkg/...

# 模块依赖图
go mod graph | grep xkit
```

### 运行时检测

```bash
# goroutine 泄漏（测试中使用 goleak.VerifyNone(t)）
go test -v -run TestXxx ./pkg/xxx/...

# benchmark
go test -bench=. -benchmem ./pkg/xxx/...
```

---

## Skills 集成

审查过程中应充分利用已安装的 Skills 进行专项评估：

| Skill | 审查用途 | 适用模块 |
|-------|---------|---------|
| `code-reviewer` | 综合代码质量审查 | 所有模块 |
| `go-style` | Go 惯用法、命名规范、接口设计 | 所有模块 |
| `golang-patterns` | Go 并发模式、错误处理模式 | 所有模块 |
| `go-test` | 测试质量、覆盖率、表驱动测试 | 所有模块 |
| `design-patterns` | 架构模式、SOLID 原则 | 架构层面 |
| `otel-go` | OpenTelemetry trace/metrics 实现 | 04-可观测性 |
| `resilience-go` | 熔断/重试/限流模式 | 05-韧性 |
| `redis-go` | Redis 使用模式、Lua 脚本 | 06-存储, 07-分布式 |
| `etcd-go` | etcd KV/Watch/Lease | 06-存储, 07-分布式 |
| `mongodb-go` | MongoDB 查询/聚合/索引 | 06-存储 |
| `clickhouse-go` | ClickHouse 表引擎/批量插入 | 06-存储 |
| `kafka-go` | Kafka 生产消费/DLQ | 08-消息队列 |
| `pulsar-go` | Pulsar 订阅模式/死信队列 | 08-消息队列 |
| `api-design-go` | API 设计规范 | 公开接口审查 |
| `backend-patterns` | 后端分层架构 | 跨模块架构审查 |

### 推荐 Skills 使用顺序

1. **`code-reviewer`** — 先做综合扫描，定位高优先级问题
2. **`go-style` + `golang-patterns`** — 惯用法和模式审查
3. **模块专属 Skill**（如 `redis-go`, `otel-go`）— 领域专项审查
4. **`go-test`** — 测试质量专项审查
5. **`design-patterns`** — 架构层面总结

---

## 新基础库原则（贯穿所有审查）

以下原则应在每个模块审查中持续验证：

1. **API 极简**：导出符号越少越好，每个导出都应有明确理由
2. **零值可用**：结构体零值应有合理行为，或通过工厂方法强制初始化
3. **接口最小化**：接口方法越少越好（1-3 个方法为佳）
4. **不设计未来**：不为"可能需要"的场景预留扩展点
5. **标准库优先**：`context.Context`, `io.Reader/Writer`, `error` 等标准接口优先
6. **错误即契约**：错误类型是 API 的一部分，应该被设计而非随意返回
7. **不兼容旧 API**：如果发现更好的设计，应直接采用，让业务方迁移
