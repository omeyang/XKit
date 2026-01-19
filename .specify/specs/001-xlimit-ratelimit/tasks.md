# xlimit 任务清单

## 元数据

| 字段 | 值 |
|------|-----|
| **Feature ID** | `feature-001-xlimit-ratelimit` |
| **关联文档** | [spec.md](./spec.md), [plan.md](./plan.md) |
| **创建日期** | 2026-01-16 |
| **状态** | Draft |
| **总任务数** | 62 |

---

## User Stories 映射

| ID | User Story | 优先级 | 任务数 |
|----|------------|--------|--------|
| US1 | 多租户限流保护 | P0 | 12 |
| US2 | API 级别细粒度限流 | P0 | 8 |
| US3 | 上游调用方限流 | P1 | 6 |
| US4 | 高可用降级保护 | P0 | 8 |
| US5 | 可观测性集成 | P1 | 10 |

---

## Phase 1: Setup（项目初始化）

**目标**：创建模块目录结构，添加依赖，初始化基础文件

- [ ] [T001] 创建模块目录 `pkg/resilience/xlimit/`
- [ ] [T002] 添加依赖 `github.com/go-redis/redis_rate/v10` 到 go.mod
- [ ] [T003] 运行 `go mod tidy` 确保依赖正确
- [ ] [T004] 创建 `pkg/resilience/xlimit/doc.go` 包文档

---

## Phase 2: Foundational（基础类型定义）

**目标**：定义核心类型和错误，无外部依赖，为后续阶段奠定基础

**独立测试标准**：所有类型可编译，错误定义可用于 errors.Is 检查

### 2.1 错误定义

- [ ] [T005] [P] 创建 `pkg/resilience/xlimit/errors.go` - 定义预定义错误
- [ ] [T006] [P] 创建 `pkg/resilience/xlimit/errors_test.go` - 错误测试

### 2.2 核心类型

- [ ] [T007] [P] 创建 `pkg/resilience/xlimit/key.go` - Key 结构和渲染方法
- [ ] [T008] [P] 创建 `pkg/resilience/xlimit/key_test.go` - Key 测试（变量替换、边界情况）
- [ ] [T009] [P] 创建 `pkg/resilience/xlimit/result.go` - Result 结构和辅助方法
- [ ] [T010] [P] 创建 `pkg/resilience/xlimit/result_test.go` - Result 测试
- [ ] [T011] [P] 创建 `pkg/resilience/xlimit/rule.go` - Rule 结构和构建器
- [ ] [T012] [P] 创建 `pkg/resilience/xlimit/rule_test.go` - Rule 测试（构建器、验证）
- [ ] [T013] [P] 创建 `pkg/resilience/xlimit/config.go` - Config 结构和默认值
- [ ] [T014] [P] 创建 `pkg/resilience/xlimit/config_test.go` - Config 测试

### 2.3 通配符匹配器

- [ ] [T015] 创建 `pkg/resilience/xlimit/internal/wildcard/matcher.go` - 通配符匹配实现
- [ ] [T016] 创建 `pkg/resilience/xlimit/internal/wildcard/matcher_test.go` - 匹配器测试

---

## Phase 3: US1 - 多租户限流保护

**User Story**：作为运维人员，我需要为不同租户配置不同的限流配额

**独立测试标准**：
- 可创建分布式限流器并执行 Allow 检查
- 租户配额相互独立
- 支持默认配额和租户覆盖

### 3.1 规则匹配器

- [ ] [T017] [US1] 创建 `pkg/resilience/xlimit/rule_matcher.go` - 规则匹配器接口和实现
- [ ] [T018] [US1] 创建 `pkg/resilience/xlimit/rule_matcher_test.go` - 匹配器测试（覆盖优先级）

### 3.2 分布式限流器

- [ ] [T019] [US1] 创建 `pkg/resilience/xlimit/limiter.go` - Limiter 接口定义
- [ ] [T020] [US1] 创建 `pkg/resilience/xlimit/distributed.go` - 分布式限流器实现（封装 redis_rate）
- [ ] [T021] [US1] 创建 `pkg/resilience/xlimit/distributed_test.go` - 分布式限流器测试（Mock Redis）

### 3.3 Options 和工厂

- [ ] [T022] [US1] 创建 `pkg/resilience/xlimit/options.go` - Option 函数定义
- [ ] [T023] [US1] 创建 `pkg/resilience/xlimit/options_test.go` - Options 测试

### 3.4 预定义规则

- [ ] [T024] [US1] 在 `rule.go` 中添加 TenantRule() 预定义规则构建器
- [ ] [T025] [US1] 在 `rule_test.go` 中添加 TenantRule 测试

### 3.5 集成测试

- [ ] [T026] [US1] 创建 `pkg/resilience/xlimit/integration_test.go` - 租户限流集成测试（需 Redis）

### 3.6 示例

- [ ] [T027] [US1] 在 `example_test.go` 中添加 Example_tenantRateLimit

---

## Phase 4: US2 - API 级别细粒度限流

**User Story**：作为开发人员，我需要为不同 API 配置不同的限流策略

**独立测试标准**：
- 支持 Method + Path 组合限流
- 支持租户 + API 组合限流
- 写操作可配置更严格限制

### 4.1 层级限流

- [ ] [T028] [US2] 在 `distributed.go` 中实现层级限流逻辑（多规则串行检查）
- [ ] [T029] [US2] 在 `distributed_test.go` 中添加层级限流测试

### 4.2 预定义规则

- [ ] [T030] [US2] [P] 在 `rule.go` 中添加 GlobalRule() 预定义规则
- [ ] [T031] [US2] [P] 在 `rule.go` 中添加 TenantAPIRule() 预定义规则
- [ ] [T032] [US2] 在 `rule_test.go` 中添加预定义规则测试

### 4.3 HTTP 中间件

- [ ] [T033] [US2] 创建 `pkg/resilience/xlimit/middleware_options.go` - 中间件 Option
- [ ] [T034] [US2] 创建 `pkg/resilience/xlimit/key_extractor.go` - HTTP/gRPC 键提取器
- [ ] [T035] [US2] 创建 `pkg/resilience/xlimit/middleware_http.go` - HTTP 中间件实现
- [ ] [T036] [US2] 创建 `pkg/resilience/xlimit/middleware_http_test.go` - HTTP 中间件测试

### 4.4 示例

- [ ] [T037] [US2] 在 `example_test.go` 中添加 Example_httpMiddleware

---

## Phase 5: US3 - 上游调用方限流

**User Story**：作为架构师，我需要限制上游服务的调用频率

**独立测试标准**：
- 支持从 Header 提取 CallerID
- 支持 CallerID 维度限流
- 支持差异化调用方配额

### 5.1 调用方支持

- [ ] [T038] [US3] 在 `key_extractor.go` 中添加 CallerID 提取逻辑
- [ ] [T039] [US3] 创建 `pkg/resilience/xlimit/key_extractor_test.go` - 键提取器测试

### 5.2 预定义规则

- [ ] [T040] [US3] 在 `rule.go` 中添加 CallerRule() 预定义规则
- [ ] [T041] [US3] 在 `rule_test.go` 中添加 CallerRule 测试

### 5.3 gRPC 拦截器

- [ ] [T042] [US3] 创建 `pkg/resilience/xlimit/middleware_grpc.go` - gRPC 拦截器实现
- [ ] [T043] [US3] 创建 `pkg/resilience/xlimit/middleware_grpc_test.go` - gRPC 拦截器测试

### 5.4 示例

- [ ] [T044] [US3] 在 `example_test.go` 中添加 Example_callerRateLimit

---

## Phase 6: US4 - 高可用降级保护

**User Story**：作为 SRE，我需要确保 Redis 故障时限流功能有降级方案

**独立测试标准**：
- Redis 不可用时自动降级
- 支持 local/fail-open/fail-close 三种策略
- Redis 恢复后自动切换回分布式限流

### 6.1 本地限流器

- [ ] [T045] [US4] 创建 `pkg/resilience/xlimit/local.go` - 本地限流器实现（基于 sync.Map + 令牌桶）
- [ ] [T046] [US4] 创建 `pkg/resilience/xlimit/local_test.go` - 本地限流器测试

### 6.2 降级包装器

- [ ] [T047] [US4] 创建 `pkg/resilience/xlimit/fallback.go` - 降级策略包装器
- [ ] [T048] [US4] 创建 `pkg/resilience/xlimit/fallback_test.go` - 降级逻辑测试

### 6.3 工厂方法更新

- [ ] [T049] [US4] 在 `limiter.go` 中更新 New() 支持降级配置
- [ ] [T050] [US4] 在 `limiter.go` 中添加 NewLocal() 工厂方法
- [ ] [T051] [US4] 创建 `pkg/resilience/xlimit/limiter_test.go` - 工厂方法测试

### 6.4 故障注入测试

- [ ] [T052] [US4] 在 `integration_test.go` 中添加故障注入测试（模拟 Redis 故障）

### 6.5 示例

- [ ] [T053] [US4] 在 `example_test.go` 中添加 Example_fallbackStrategy

---

## Phase 7: US5 - 可观测性集成

**User Story**：作为监控人员，我需要看到限流的运行状态和触发情况

**独立测试标准**：
- Prometheus 指标正确导出
- 限流触发时记录结构化日志
- HTTP 响应包含标准限流头

### 7.1 Prometheus 指标

- [ ] [T054] [US5] 创建 `pkg/resilience/xlimit/metrics.go` - Prometheus 指标定义和注册
- [ ] [T055] [US5] 创建 `pkg/resilience/xlimit/metrics_test.go` - 指标测试

### 7.2 响应头支持

- [ ] [T056] [US5] 在 `result.go` 中添加 Headers() 和 SetHeaders() 方法
- [ ] [T057] [US5] 在 `result_test.go` 中添加响应头测试

### 7.3 日志集成

- [ ] [T058] [US5] 在 `options.go` 中添加 WithOnAllow/WithOnDeny 回调
- [ ] [T059] [US5] 在 `distributed.go` 中集成回调调用

### 7.4 配置加载器

- [ ] [T060] [US5] 创建 `pkg/resilience/xlimit/config_loader.go` - 配置加载器接口
- [ ] [T061] [US5] 创建 `pkg/resilience/xlimit/config_loader_file.go` - 文件配置加载器
- [ ] [T062] [US5] 创建 `pkg/resilience/xlimit/config_loader_etcd.go` - etcd 配置加载器（热更新）

### 7.5 示例

- [ ] [T063] [US5] 在 `example_test.go` 中添加 Example_dynamicConfig

---

## Phase 8: Polish（收尾和交叉关注点）

**目标**：完善文档、基准测试、代码质量检查

### 8.1 文档完善

- [ ] [T064] 更新 `doc.go` 包文档，添加完整使用说明
- [ ] [T065] 整理 `example_test.go`，确保所有示例可运行
- [ ] [T066] 检查所有导出函数/类型的中文注释

### 8.2 基准测试

- [ ] [T067] 创建 `pkg/resilience/xlimit/benchmark_test.go` - 基准测试
- [ ] [T068] 验证 P99 < 5ms 目标

### 8.3 代码质量

- [ ] [T069] 运行 `golangci-lint run ./pkg/resilience/xlimit/...` 修复所有问题
- [ ] [T070] 运行 `go test -race ./pkg/resilience/xlimit/...` 确保无数据竞争
- [ ] [T071] 运行 `go test -cover ./pkg/resilience/xlimit/...` 确保覆盖率达标

### 8.4 最终验收

- [ ] [T072] 运行完整测试套件 `task test`
- [ ] [T073] 更新 README.md 添加 xlimit 模块说明

---

## 依赖关系图

```
Phase 1 (Setup)
    │
    ▼
Phase 2 (Foundational) ──────────────────────────────────┐
    │                                                     │
    ▼                                                     │
Phase 3 (US1: 多租户) ◄───────────────────────────────────┤
    │                                                     │
    ├──────────────────┐                                  │
    ▼                  ▼                                  │
Phase 4 (US2: API)   Phase 5 (US3: 调用方)               │
    │                  │                                  │
    └────────┬─────────┘                                  │
             ▼                                            │
      Phase 6 (US4: 降级) ◄────────────────────────────────┤
             │                                            │
             ▼                                            │
      Phase 7 (US5: 可观测) ◄──────────────────────────────┘
             │
             ▼
      Phase 8 (Polish)
```

---

## 并行执行机会

### Phase 2 内部并行

以下任务可并行执行（不同文件，无依赖）：

```
并行组 A:
├── T005 errors.go
├── T007 key.go
├── T009 result.go
├── T011 rule.go
└── T013 config.go

并行组 B（依赖组 A）:
├── T006 errors_test.go
├── T008 key_test.go
├── T010 result_test.go
├── T012 rule_test.go
└── T014 config_test.go
```

### Phase 4 内部并行

```
并行组:
├── T030 GlobalRule()
└── T031 TenantAPIRule()
```

### 跨 Phase 并行

Phase 4 (US2) 和 Phase 5 (US3) 可并行开发：
- US2 专注 HTTP 中间件
- US3 专注 gRPC 拦截器

---

## 实现策略

### MVP 范围（最小可行产品）

**建议 MVP 仅包含 Phase 1-3（US1）**：

- 完成基础类型定义
- 完成分布式限流器核心功能
- 完成租户级限流
- 可独立测试和发布

### 增量交付顺序

```
MVP (v0.1.0)
└── Phase 1-3: 租户限流核心

迭代 1 (v0.2.0)
└── Phase 4: API 级别限流 + HTTP 中间件

迭代 2 (v0.3.0)
├── Phase 5: 调用方限流 + gRPC 拦截器
└── Phase 6: 降级保护

迭代 3 (v1.0.0)
├── Phase 7: 可观测性
└── Phase 8: 收尾
```

---

## 任务统计

| Phase | 任务数 | 可并行 |
|-------|--------|--------|
| Phase 1: Setup | 4 | 0 |
| Phase 2: Foundational | 12 | 10 |
| Phase 3: US1 | 11 | 0 |
| Phase 4: US2 | 10 | 2 |
| Phase 5: US3 | 7 | 0 |
| Phase 6: US4 | 9 | 0 |
| Phase 7: US5 | 10 | 0 |
| Phase 8: Polish | 10 | 0 |
| **总计** | **73** | **12** |

---

## 验收检查清单

### 功能验收

- [ ] 分布式限流正常工作（US1）
- [ ] 层级限流正常工作（US2）
- [ ] HTTP 中间件正常工作（US2）
- [ ] gRPC 拦截器正常工作（US3）
- [ ] 降级策略正常工作（US4）
- [ ] Prometheus 指标正常导出（US5）
- [ ] 动态配置热更新正常工作（US5）

### 质量验收

- [ ] 测试覆盖率（核心）≥ 95%
- [ ] 测试覆盖率（整体）≥ 90%
- [ ] golangci-lint 零错误
- [ ] go test -race 无竞争
- [ ] 基准测试 P99 < 5ms

### 文档验收

- [ ] doc.go 包文档完整
- [ ] example_test.go 示例完整
- [ ] 所有导出 API 有中文注释
