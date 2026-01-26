# xdbg 任务清单

## 元数据

| 字段 | 值 |
|------|-----|
| **Feature ID** | `feature-002-xdbg-runtime-debug` |
| **关联文档** | [spec.md](./spec.md), [plan.md](./plan.md), [decisions.md](./decisions.md) |
| **创建日期** | 2026-01-22 |
| **状态** | Draft |
| **总任务数** | 78 |

---

## User Stories 映射

| ID | User Story | 优先级 | 任务数 |
|----|------------|--------|--------|
| US1 | 动态修改日志级别 | P0 | 8 |
| US2 | 生产环境性能分析 | P0 | 10 |
| US3 | 查看弹性组件状态 | P1 | 8 |
| US4 | 安全的调试访问 | P0 | 10 |
| US5 | K8s 环境调试（客户端） | P0 | 12 |

---

## Phase 1: Setup（项目初始化）

**目标**：创建模块目录结构，初始化基础文件

- [ ] [T001] 创建模块目录 `pkg/debug/xdbg/`
- [ ] [T002] 创建客户端目录 `cmd/xdbgctl/`
- [ ] [T003] 创建 `pkg/debug/xdbg/doc.go` 包文档
- [ ] [T004] 运行 `go mod tidy` 确保依赖正确

---

## Phase 2: Foundational（基础类型定义）

**目标**：定义核心类型、协议和错误，无外部依赖，为后续阶段奠定基础

**独立测试标准**：所有类型可编译，协议编解码可通过单元测试

### 2.1 错误定义

- [ ] [T005] [P] 创建 `pkg/debug/xdbg/errors.go` - 定义预定义错误（ErrNotRunning, ErrCommandNotFound, ErrTimeout 等）
- [ ] [T006] [P] 创建 `pkg/debug/xdbg/errors_test.go` - 错误测试（errors.Is 检查）

### 2.2 协议定义

- [ ] [T007] [P] 创建 `pkg/debug/xdbg/protocol.go` - 协议常量和消息类型定义
- [ ] [T008] [P] 创建 `pkg/debug/xdbg/protocol_codec.go` - 编解码器（Header + JSON Payload）
- [ ] [T009] [P] 创建 `pkg/debug/xdbg/protocol_codec_test.go` - 编解码测试（边界情况、大消息）

### 2.3 Command 接口

- [ ] [T010] [P] 创建 `pkg/debug/xdbg/command.go` - Command 接口定义
- [ ] [T011] [P] 创建 `pkg/debug/xdbg/command_registry.go` - 命令注册表实现
- [ ] [T012] [P] 创建 `pkg/debug/xdbg/command_registry_test.go` - 注册表测试

---

## Phase 3: US4 - 安全的调试访问（核心基础设施）

**User Story**：作为安全工程师，我需要确保调试功能不会成为安全漏洞

**独立测试标准**：
- 触发器可监听信号并发送事件
- Unix Socket 可创建、监听、接受连接
- SO_PEERCRED 可获取调用者身份
- 审计日志可记录操作

### 3.1 触发器

- [ ] [T013] [US4] 创建 `pkg/debug/xdbg/trigger.go` - Trigger 接口定义
- [ ] [T014] [US4] 创建 `pkg/debug/xdbg/trigger_signal.go` - 信号触发器实现（SIGUSR1）
- [ ] [T015] [US4] 创建 `pkg/debug/xdbg/trigger_signal_windows.go` - Windows stub（不支持）
- [ ] [T016] [US4] 创建 `pkg/debug/xdbg/trigger_test.go` - 触发器测试

### 3.2 传输层

- [ ] [T017] [US4] 创建 `pkg/debug/xdbg/transport.go` - Transport 接口定义
- [ ] [T018] [US4] 创建 `pkg/debug/xdbg/transport_unix.go` - Unix Socket 实现（文件权限 0600）
- [ ] [T019] [US4] 创建 `pkg/debug/xdbg/transport_unix_test.go` - Unix Socket 测试

### 3.3 身份识别与审计

- [ ] [T020] [US4] 创建 `pkg/debug/xdbg/identity.go` - SO_PEERCRED 获取调用者 UID/PID
- [ ] [T021] [US4] 创建 `pkg/debug/xdbg/identity_test.go` - 身份识别测试
- [ ] [T022] [US4] 创建 `pkg/debug/xdbg/audit.go` - 审计日志记录器
- [ ] [T023] [US4] 创建 `pkg/debug/xdbg/audit_test.go` - 审计日志测试

---

## Phase 4: Server 核心（服务器生命周期）

**目标**：实现 Server 核心，支持启动、停止、自动关闭

**独立测试标准**：
- Server 可启动和停止
- 支持自动关闭（可配置超时）
- 支持并发限制

### 4.1 Options 配置

- [ ] [T024] 创建 `pkg/debug/xdbg/options.go` - Option 函数定义
- [ ] [T025] 创建 `pkg/debug/xdbg/options_test.go` - Options 测试

### 4.2 Server 实现

- [ ] [T026] 创建 `pkg/debug/xdbg/server.go` - Server 核心实现
- [ ] [T027] 创建 `pkg/debug/xdbg/session.go` - 会话管理（并发限制、命令执行）
- [ ] [T028] 创建 `pkg/debug/xdbg/server_test.go` - Server 生命周期测试
- [ ] [T029] 创建 `pkg/debug/xdbg/session_test.go` - 会话测试

### 4.3 资源管理测试

- [ ] [T030] 创建 `pkg/debug/xdbg/leak_test.go` - goleak + FD 泄露测试

---

## Phase 5: US1 - 动态修改日志级别

**User Story**：作为 SRE，我需要在生产环境临时开启 Debug 日志排查问题

**独立测试标准**：
- setlog 命令可修改日志级别
- 与 xlog.Leveler 接口集成
- help/exit 命令正常工作

### 5.1 基础命令

- [ ] [T031] [US1] 创建 `pkg/debug/xdbg/command_builtin.go` - help/exit 命令实现
- [ ] [T032] [US1] 创建 `pkg/debug/xdbg/command_builtin_test.go` - 基础命令测试

### 5.2 setlog 命令

- [ ] [T033] [US1] 在 `command_builtin.go` 中添加 setlog 命令
- [ ] [T034] [US1] 定义 `Leveler` 接口（与 xlog 兼容）
- [ ] [T035] [US1] 在 `options.go` 中添加 `WithLogger(Leveler)` Option
- [ ] [T036] [US1] 在 `command_builtin_test.go` 中添加 setlog 测试

### 5.3 集成测试

- [ ] [T037] [US1] 创建 `pkg/debug/xdbg/integration_test.go` - setlog 集成测试
- [ ] [T038] [US1] 在 `example_test.go` 中添加 Example_setlog

---

## Phase 6: US2 - 生产环境性能分析

**User Story**：作为开发人员，我需要在生产环境采集 CPU/Memory profile 分析性能问题

**独立测试标准**：
- pprof cpu start/stop 可采集 CPU profile
- pprof heap 可导出堆内存
- stack 可打印 goroutine 堆栈
- freemem 可释放内存

### 6.1 pprof 命令

- [ ] [T039] [US2] 在 `command_builtin.go` 中添加 pprof cpu start 命令
- [ ] [T040] [US2] 在 `command_builtin.go` 中添加 pprof cpu stop 命令
- [ ] [T041] [US2] 在 `command_builtin.go` 中添加 pprof heap 命令
- [ ] [T042] [US2] 在 `command_builtin.go` 中添加 pprof goroutine 命令
- [ ] [T043] [US2] 在 `command_builtin_test.go` 中添加 pprof 测试

### 6.2 stack 命令

- [ ] [T044] [US2] 在 `command_builtin.go` 中添加 stack 命令
- [ ] [T045] [US2] 在 `command_builtin_test.go` 中添加 stack 测试

### 6.3 freemem 命令

- [ ] [T046] [US2] 在 `command_builtin.go` 中添加 freemem 命令
- [ ] [T047] [US2] 在 `command_builtin_test.go` 中添加 freemem 测试

### 6.4 示例

- [ ] [T048] [US2] 在 `example_test.go` 中添加 Example_pprof

---

## Phase 7: US3 - 查看弹性组件状态

**User Story**：作为运维人员，我需要查看熔断器、限流器的实时状态

**独立测试标准**：
- breaker 命令可查看熔断器状态
- limit 命令可查看限流器状态
- cache 命令可查看缓存统计
- config 命令可查看运行时配置

### 7.1 xkit 集成接口

- [ ] [T049] [US3] 创建 `pkg/debug/xdbg/command_xkit.go` - xkit 集成命令框架
- [ ] [T050] [US3] 定义 `BreakerRegistry` 接口（与 xbreaker 兼容）
- [ ] [T051] [US3] 定义 `LimiterRegistry` 接口（与 xlimit 兼容）

### 7.2 集成命令

- [ ] [T052] [US3] 在 `command_xkit.go` 中添加 breaker 命令
- [ ] [T053] [US3] 在 `command_xkit.go` 中添加 limit 命令
- [ ] [T054] [US3] 在 `command_xkit.go` 中添加 cache 命令（P2）
- [ ] [T055] [US3] 在 `command_xkit.go` 中添加 config 命令（P2）

### 7.3 测试

- [ ] [T056] [US3] 创建 `pkg/debug/xdbg/command_xkit_test.go` - xkit 集成命令测试

---

## Phase 8: US5 - K8s 环境调试（客户端）

**User Story**：作为 K8s 运维，我需要安全的调试触发方式

**独立测试标准**：
- xdbgctl enable/disable 可控制调试服务
- xdbgctl setlog debug 可修改日志级别
- 支持单命令模式和交互模式
- 静态编译，无外部依赖

### 8.1 客户端核心

- [ ] [T057] [US5] 添加 `github.com/urfave/cli/v3` 依赖
- [ ] [T058] [US5] 创建 `cmd/xdbgctl/client.go` - 客户端通信实现
- [ ] [T059] [US5] 创建 `cmd/xdbgctl/client_test.go` - 客户端测试

### 8.2 CLI 命令

- [ ] [T060] [US5] 创建 `cmd/xdbgctl/main.go` - 入口和全局选项
- [ ] [T061] [US5] 创建 `cmd/xdbgctl/commands.go` - 子命令定义
- [ ] [T062] [US5] 在 `commands.go` 中添加 enable/disable 命令
- [ ] [T063] [US5] 在 `commands.go` 中添加 setlog 命令
- [ ] [T064] [US5] 在 `commands.go` 中添加 stack/pprof 命令
- [ ] [T065] [US5] 在 `commands.go` 中添加 breaker/limit 命令

### 8.3 交互模式

- [ ] [T066] [US5] 在 `commands.go` 中添加 interactive 命令（REPL）
- [ ] [T067] [US5] 创建 `cmd/xdbgctl/interactive.go` - 交互模式实现

### 8.4 构建验证

- [ ] [T068] [US5] 验证静态编译：`CGO_ENABLED=0 go build -o xdbgctl ./cmd/xdbgctl`

---

## Phase 9: Polish（收尾和交叉关注点）

**目标**：完善文档、基准测试、代码质量检查

### 9.1 文档完善

- [ ] [T069] 更新 `pkg/debug/xdbg/doc.go` 包文档，添加完整使用说明
- [ ] [T070] 整理 `example_test.go`，确保所有示例可运行
- [ ] [T071] 检查所有导出函数/类型的中文注释

### 9.2 基准测试

- [ ] [T072] 创建 `pkg/debug/xdbg/benchmark_test.go` - 基准测试
- [ ] [T073] 验证命令响应 P99 < 50ms 目标
- [ ] [T074] 验证未激活时内存 < 1MB 目标

### 9.3 代码质量

- [ ] [T075] 运行 `golangci-lint run ./pkg/debug/xdbg/...` 修复所有问题
- [ ] [T076] 运行 `go test -race ./pkg/debug/xdbg/...` 确保无数据竞争
- [ ] [T077] 运行 `go test -cover ./pkg/debug/xdbg/...` 确保覆盖率达标

### 9.4 最终验收

- [ ] [T078] 运行完整测试套件 `task test`

---

## 依赖关系图

```
Phase 1 (Setup)
    │
    ▼
Phase 2 (Foundational: 协议+命令接口)
    │
    ▼
Phase 3 (US4: 安全基础设施) ──────────────┐
    │                                      │
    ▼                                      │
Phase 4 (Server 核心)                      │
    │                                      │
    ├────────────┬────────────┐            │
    ▼            ▼            ▼            │
Phase 5      Phase 6      Phase 7          │
(US1:日志)   (US2:pprof)  (US3:xkit)       │
    │            │            │            │
    └────────────┴────────────┘            │
                 │                         │
                 ▼                         │
          Phase 8 (US5: 客户端) ◄──────────┘
                 │
                 ▼
          Phase 9 (Polish)
```

---

## 并行执行机会

### Phase 2 内部并行

以下任务可并行执行（不同文件，无依赖）：

```
并行组 A:
├── T005 errors.go
├── T007 protocol.go
└── T010 command.go

并行组 B（依赖组 A）:
├── T006 errors_test.go
├── T008 protocol_codec.go
├── T011 command_registry.go

并行组 C（依赖组 B）:
├── T009 protocol_codec_test.go
└── T012 command_registry_test.go
```

### Phase 3 内部并行

```
并行组:
├── T013-T016 触发器（独立）
├── T017-T019 传输层（独立）
└── T020-T023 身份+审计（独立）
```

### Phase 5/6/7 跨 Phase 并行

Phase 5 (US1)、Phase 6 (US2)、Phase 7 (US3) 可并行开发：
- US1 专注 setlog 命令
- US2 专注 pprof/stack/freemem 命令
- US3 专注 xkit 集成命令

---

## 实现策略

### MVP 范围（最小可行产品）

**建议 MVP 包含 Phase 1-5（US1 + US4 核心）**：

- 完成协议和基础类型定义
- 完成安全基础设施（触发器、传输层、审计）
- 完成 Server 核心
- 完成 setlog 基础命令
- 可独立测试和发布

### 增量交付顺序

```
MVP (v0.1.0)
└── Phase 1-5: 核心 + setlog

迭代 1 (v0.2.0)
└── Phase 6: pprof/stack/freemem

迭代 2 (v0.3.0)
└── Phase 7: xkit 集成命令

迭代 3 (v0.4.0)
└── Phase 8: xdbgctl 客户端

迭代 4 (v1.0.0)
└── Phase 9: 收尾
```

---

## 任务统计

| Phase | 任务数 | 可并行 |
|-------|--------|--------|
| Phase 1: Setup | 4 | 0 |
| Phase 2: Foundational | 8 | 6 |
| Phase 3: US4 安全基础 | 11 | 9 |
| Phase 4: Server 核心 | 7 | 0 |
| Phase 5: US1 日志 | 8 | 0 |
| Phase 6: US2 性能分析 | 10 | 0 |
| Phase 7: US3 xkit集成 | 8 | 0 |
| Phase 8: US5 客户端 | 12 | 0 |
| Phase 9: Polish | 10 | 0 |
| **总计** | **78** | **15** |

---

## 验收检查清单

### 功能验收

- [ ] 信号触发正常（开关模式）（US4）
- [ ] 命令触发正常（enable/disable）（US5）
- [ ] Unix Socket 通信正常（US4）
- [ ] 自动关闭正常（US4）
- [ ] setlog 命令正常（US1）
- [ ] pprof/stack/freemem 命令正常（US2）
- [ ] breaker/limit 命令正常（US3）
- [ ] xdbgctl 客户端正常（US5）

### 质量验收

- [ ] 测试覆盖率（核心）≥ 95%
- [ ] 测试覆盖率（整体）≥ 90%
- [ ] golangci-lint 零错误
- [ ] go test -race 无竞争
- [ ] 命令响应 P99 < 50ms
- [ ] 未激活时内存 < 1MB

### 资源安全验收

- [ ] goleak 测试通过
- [ ] FD 计数测试通过
- [ ] 优雅关闭测试通过
- [ ] Context 取消传播正确
- [ ] Socket 文件正确清理

### 文档验收

- [ ] doc.go 包文档完整
- [ ] example_test.go 示例完整
- [ ] 所有导出 API 有中文注释
