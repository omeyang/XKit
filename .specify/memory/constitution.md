# XKit Constitution

XKit 是新一代 Go 基础工具库项目原则文档。本文档定义项目的核心原则、技术约束和治理规则。

---

## 核心原则

### I. 模块化设计（强制）

**原则**：每个功能必须是独立、可复用的模块，具有清晰的职责边界。

**要求**：
- 每个包（package）只负责一个明确的功能领域
- 包之间依赖关系清晰，避免循环依赖
- 公开 API（pkg/）和内部实现（internal/）严格分离
- 每个模块必须有独立的单元测试

**验证方法**：
- 使用 `go mod graph` 检查依赖关系
- Code Review 时检查包的职责单一性
- 测试覆盖率 ≥ 80%

---

### II. API 稳定性优先（强制）

**原则**：公开 API（pkg/）必须保持向后兼容，破坏性变更需要遵循语义化版本控制。

**要求**：
- 使用语义化版本控制（MAJOR.MINOR.PATCH）
- 破坏性变更（Breaking Changes）必须：
  - 提升 MAJOR 版本号
  - 在 CHANGELOG.md 中明确标注
  - 提供迁移指南
  - 通过 ADR 记录决策
- 新增功能提升 MINOR 版本号
- Bug 修复提升 PATCH 版本号

**验证方法**：
- 使用 `go test -run=TestBackwardCompatibility` 验证兼容性
- Code Review 时检查 API 变更影响
- 版本发布前运行兼容性测试套件

---

### III. 测试优先开发（强制）

**原则**：所有功能必须先编写测试，测试通过后才能合并代码。遵循 TDD（Test-Driven Development）红-绿-重构循环。

**TDD 三阶段**：
1. **红（Red）**：先写测试，运行测试确保失败（因为功能还未实现）
2. **绿（Green）**：编写最小化代码使测试通过
3. **重构（Refactor）**：在测试保护下优化代码结构

**要求**：
- **测试覆盖率**（强制）：
  - **核心业务逻辑**：≥ 95%
  - **整体代码**：≥ 90%
  - 使用 `go test -cover` 验证
- **测试结构**：
  - 优先使用 **表驱动测试**（table-driven tests）
  - 测试命名遵循 `TestXxx_Given_When_Then` 模式
  - 测试代码使用 **Given-When-Then** 结构
- **Mock 策略**：
  - **MUST Mock**：外部依赖（数据库、HTTP、文件系统、时间）
  - **MUST NOT Mock**：核心业务逻辑和领域对象
  - 使用 `go.uber.org/mock` 生成 Mock
- **基准测试**：性能关键路径必须有 benchmark
- **示例测试**：公开 API 必须有 Example 测试（用于生成文档）
- **测试隔离**：每个测试独立运行，不依赖外部状态

**验证方法**：
```bash
# 运行测试（推荐使用 go-task）
task test

# 测试覆盖率
task test-cover

# 基准测试
go test -bench=. ./...

# 数据竞争检测
task test-race
```

**参考文档**：`.specify/prior-art/01-standards/03-testing/02-testing-standards.md`

---

### IV. 代码质量标准（强制）

**原则**：所有代码必须通过 golangci-lint v2 检查，遵循 Go 最佳实践。

**要求**：
- **Lint 检查**：golangci-lint run 必须通过（零错误、零警告）
- **禁止跳过 Lint**：生产代码（非 `_test.go`）**禁止**使用 `//nolint` 或 `//nolintlint`，必须显式修复或重构；如确需例外，必须在 `.specify/memory/01-exceptions.md` 记录并设定有效期
- **代码格式化**：使用 gofmt 和 goimports
- **注释规范**：
  - 公开 API 必须有中文注释（用于团队理解）
  - 注释必须描述"为什么"而非"是什么"
  - 每个包必须有 package doc（doc.go）
- **错误处理**：
  - 所有错误必须被处理或明确忽略
  - 使用 `fmt.Errorf` 包装错误，提供上下文信息
  - 禁止使用 `panic`（除非是不可恢复的错误）

**验证方法**：
```bash
# Lint 检查
golangci-lint run ./...

# 格式化检查
gofmt -l .
goimports -l .
```

---

### V. 文档金标准（强制）

**原则**：所有技术文档必须遵循文档金标准（12 个核心原则）。

**要求**：
- **适用范围**：
  - `.specify/specs/*/analysis/` - 技术分析文档
  - `.specify/specs/*/design/` - 设计文档
  - `.specify/prior-art/` - 先验知识文档、项目文档
- **核心要求**：
  - 单一职责：每个文档只负责一个主题
  - 图表优先：代码块 ≤ 5 行，超过改用 Mermaid
  - 专业术语：删除口语化和自问自答
  - 数据驱动：所有结论有数据支撑
  - 真实可靠：基于实际代码和数据，不猜测
  - 时效准确：只记录最新状态
  - 关联准确：所有引用和链接有效
  - 代码溯源：标注仓库、分支、提交
  - 决策留痕：记录已拒绝方案（ADR）
  - 文档长度：单文件 ≤ 800 行
- **参考文档**：`.specify/prior-art/01-standards/01-documentation/`

**验证方法**：
- Code Review 时检查文档质量
- 使用文档检查清单（12 个核心原则）

---

### VI. 性能与资源效率

**原则**：基础库必须高效，避免不必要的内存分配和 CPU 消耗。

**要求**：
- **基准测试**：性能关键路径必须有 benchmark
- **内存分配**：
  - 使用 `testing.B.ReportAllocs()` 监控内存分配
  - 热路径代码避免不必要的内存分配
  - 优先使用对象池（sync.Pool）复用对象
- **并发安全**：
  - 并发访问的数据结构必须是线程安全的
  - 使用 `go test -race` 检测数据竞争
  - 明确标注是否并发安全

**验证方法**：
```bash
# 基准测试
go test -bench=. -benchmem ./...

# 数据竞争检测
go test -race ./...
```

---

### VII. 依赖管理

**原则**：最小化外部依赖，所有依赖必须经过审核。

**要求**：
- **依赖原则**：
  - 优先使用标准库
  - 引入外部依赖必须通过 ADR 记录决策
  - 定期审查依赖的安全性和维护状态
- **版本锁定**：
  - 使用 `go.mod` 锁定依赖版本
  - 使用 `go.sum` 验证依赖完整性
- **内网代理**：
  - 使用深信为内网 Go 代理：`http://mirrors.sangfor.com/nexus/repository/go-proxy/`

**验证方法**：
```bash
# 查看依赖
go list -m all

# 依赖图
go mod graph

# 更新依赖
go get -u ./...
go mod tidy
```

---

## 技术约束

### Go 版本

**固定版本**：Go 1.23.12（流水线要求）

**配置位置**：
- `go.mod` 中 `go` 指令：`go 1.23.12`

**验证方法**：
```bash
go version  # 确认运行时版本
grep "^go " go.mod  # 确认 go.mod 版本
```

### 代码组织

**目录结构**：
```
xkit/
├── internal/       # 内部包（不导出，仅项目内使用）
├── pkg/            # 公开包（可导出，供外部使用）
├── examples/       # 使用示例
└── .specify/
    └── prior-art/  # 项目文档、先验知识
```

**命名规范**：
- 包名：小写，单数形式，简短（如 `http`, `json`）
- 文件名：snake_case（如 `user_service.go`）
- 变量/函数：camelCase（如 `getUserByID`）
- 常量：CamelCase（如 `MaxRetries`）

### 代码风格

**格式化**：
- 使用 `gofmt` 格式化代码
- 使用 `goimports` 管理导入

**注释**：
- 公开 API 必须有注释（中文）
- 注释格式：`// FunctionName 描述`
- 包注释：在 `doc.go` 中

---

## 开发工作流

### Spec-Driven Development

**强制流程**：所有新功能必须遵循 Spec-Driven Development 工作流

**流程**：
```
Phase 0: /speckit.constitution - 建立项目原则
  ↓
Phase 0.5: /speckit.search - 🚨 搜索已有方案（强制，防止重复创造）
  ↓
Phase 1: /speckit.specify - 需求规格（10项强制字段）
  ↓
Phase 1.5: /speckit.clarify - 澄清需求
  ↓
Phase 2: /speckit.plan - 技术计划
  ↓
Phase 3: /speckit.tasks - 任务拆解
  ↓
Phase 3.5: /speckit.analyze - 一致性分析
  ↓
Phase 4: /speckit.implement - 执行实现
  ↓
Phase 5: /speckit.archive-session - 会话归档
```

**检查清单**：
- [ ] Phase 0.5 搜索已有方案（相似度 ≥ 80% 禁止创建）
- [ ] Spec 包含 10 项强制字段 + 人工审核签字
- [ ] Plan 包含 ADR 记录技术决策
- [ ] Tasks 拆解为可执行任务
- [ ] 会话日志归档到 `.specify/specs/{feature}/ai-sessions/`

### Git 提交规范

**Commit 消息**：
- 使用全局 Git Commit 模板（`~/.gitmessage`）
- 包含 AI-Meta 信息（模型、会话ID、生成占比、审核人）
- **禁止 Co-Authored-By**：提交消息中**禁止**包含 `Co-Authored-By` 签名行（如 `Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>`）。AI 辅助信息应通过 AI-Meta 字段记录，而非 Co-Authored-By

**Merge Request**：
- 使用 `.gitlab/merge_request_templates/ai-assisted.md` 模板
- 关联 Spec、Plan、Tasks 文档
- 提供测试证据和验收确认

### Code Review 要求

**检查要点**：
- [ ] 代码符合项目原则（本文档）
- [ ] 测试覆盖率达标（核心业务 ≥ 95%，整体 ≥ 90%）
- [ ] golangci-lint 检查通过
- [ ] 文档完整（代码注释 + 技术文档）
- [ ] 无明显性能问题
- [ ] 错误处理完善
- [ ] 并发安全（如适用）

**审核人**：
- 架构师：技术选型、API 设计
- 安全专家：安全漏洞、敏感数据处理
- 测试经理：测试覆盖率、测试用例质量

---

## 质量门禁

### 提交前检查

**本地检查**（必须通过）：
```bash
# 推荐使用 go-task
task pre-commit  # 快速检查
task ci          # 完整检查

# 或手动执行
# 1. 格式化检查
task fmt-check

# 2. Lint 检查
task lint

# 3. 测试
task test

# 4. 测试覆盖率（核心业务 ≥ 95%，整体 ≥ 90%）
task test-cover

# 5. 数据竞争检测
task test-race
```

### MR 合并前检查

**流水线检查**（必须通过）：
- [ ] 编译通过
- [ ] 单元测试通过
- [ ] 测试覆盖率达标（核心业务 ≥ 95%，整体 ≥ 90%）
- [ ] golangci-lint 检查通过
- [ ] 数据竞争检测通过
- [ ] 安全扫描通过

**人工审核**（必须通过）：
- [ ] 至少 1 名架构师审核
- [ ] Code Review 通过（无未解决的评论）
- [ ] 验收标准全部满足

---

## 治理规则

### 原则优先级

**优先级**：
1. 本 Constitution 文档优先级最高
2. 与本文档冲突的其他规范，以本文档为准
3. 本文档未覆盖的领域，参考业界最佳实践

### 修订流程

**修订原则**：
- Constitution 修订必须通过 ADR 记录决策
- 修订必须经过团队讨论和投票
- 修订生效前必须提供迁移计划（如适用）

**修订步骤**：
1. 提交修订提案（ADR 格式）
2. 团队讨论（至少 3 个工作日）
3. 投票决策（超过 2/3 成员同意）
4. 更新 Constitution 文档
5. 发布修订公告
6. 执行迁移计划（如适用）

### 例外处理

**例外申请**：
- 遇到特殊情况需要偏离 Constitution 时，必须提交例外申请
- 例外申请必须包含：
  - 例外原因和背景
  - 风险评估
  - 缓解措施
  - 持续时间（临时例外必须有明确的结束时间）
- 例外申请必须经过架构师审批

**记录要求**：
- 所有例外决策记录在 `.specify/memory/01-exceptions.md`
- 临时例外到期后必须恢复合规

---

## 参考文档

**项目内文档**：
- **文档金标准**: `.specify/prior-art/01-standards/01-documentation/`
- **测试标准**: `.specify/prior-art/01-standards/03-testing/02-testing-standards.md`
- **代码质量标准**: `.specify/prior-art/01-standards/02-code-quality/`
- **Go 依赖标准**: `.specify/prior-art/01-standards/04-go-ecosystem/01-go-dependencies.md`
- **标准文档索引**: `.specify/prior-art/README.md`
