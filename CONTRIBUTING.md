# 贡献指南

欢迎为 XKit 项目做贡献！本文档提供贡献流程和规范说明。

---

## 开发环境准备

### 必需工具

- **Go 1.25.4**（固定版本，流水线要求）
- **golangci-lint v2**（代码质量检查）
- **go-task**（任务运行器，替代 Makefile）
- **Git**（版本控制）

### 验证环境

```bash
# 检查 Go 版本
go version  # 应显示 go1.25.4

# 检查 golangci-lint
golangci-lint version

# 克隆项目
git clone git@github.com:omeyang/XKit.git
cd XKit

# 安装依赖
go mod download
```

---

## 开发工作流

### Spec-Driven Development 流程

本项目遵循 **Spec-Driven Development** 工作流，所有新功能必须按以下流程开发：

```
Phase 0: 项目原则 (.specify/memory/constitution.md)
  ↓
Phase 1: 需求规格 (/speckit.specify) - 包含 10 项强制字段
  ↓
Phase 1.5: 澄清需求 (/speckit.clarify)
  ↓
Phase 2: 技术计划 (/speckit.plan) - 包含 ADR
  ↓
Phase 3: 任务拆解 (/speckit.tasks)
  ↓
Phase 3.5: 一致性分析 (/speckit.analyze)
  ↓
Phase 4: 执行实现 (/speckit.implement)
```

### 创建新功能

**步骤 1: 创建分支**

```bash
git checkout -b feature/001-feature-name
```

**步骤 2: Phase 1 - 创建需求规格**

使用 `/speckit.specify` 命令创建需求规格，必须包含：
- 10 项强制字段（元数据、需求溯源、差异化等）
- SMART 验收标准（可量化、可测试）
- 人工审核签字（架构师、安全专家、测试经理）

**步骤 3: Phase 2-4 - 技术计划、任务拆解、执行实现**

依次执行后续 Phase，确保：
- Plan 包含 ADR 记录技术决策
- Tasks 拆解为可执行任务
- 实现符合项目原则（constitution.md）

**步骤 4:（可选）会话归档**

如需保留 AI 会话日志，可手动归档到 `.specify/specs/{feature}/ai-sessions/`

---

## 代码规范

### 目录组织

```text
xkit/
├── internal/       # 内部包（不导出，仅项目内使用）
├── pkg/            # 公开包（可导出，供外部使用）
└── .specify/       # 规格、标准与项目记忆
```

### 命名规范

- **包名**：小写，单数形式，简短（如 `http`, `json`）
- **文件名**：snake_case（如 `user_service.go`）
- **变量/函数**：camelCase（如 `getUserByID`）
- **常量**：CamelCase（如 `MaxRetries`）

### 注释规范

**公开 API 必须有中文注释**：

```go
// GetUser 根据用户 ID 获取用户信息
// 返回用户对象和错误信息
func GetUser(id int) (*User, error) {
    // 实现...
}
```

**每个包必须有 package doc**（doc.go）：

```go
// Package http 提供 HTTP 客户端和服务端功能
//
// 主要功能：
// - HTTP 客户端封装
// - HTTP 服务端中间件
// - 请求/响应处理工具
package http
```

### Lint 规则与排除

- `.golangci.yml` 中的排除项应保持最小化，优先在代码处使用 `//nolint:<linter>` 并说明原因
- 新增全局排除必须在 PR 中说明原因，并同步更新此文档

---

## 测试规范

### TDD 开发模式

本项目遵循 **Test-Driven Development**（测试驱动开发）模式：

**三阶段循环**：
1. **红（Red）**：先写测试，运行测试确保失败（功能未实现）
2. **绿（Green）**：编写最小化代码使测试通过
3. **重构（Refactor）**：在测试保护下优化代码结构

### 测试覆盖率

**要求**：
- **核心业务逻辑**：≥ 95%
- **整体代码**：≥ 90%

**运行测试**：

```bash
# 运行所有测试
task test

# 测试覆盖率（生成 HTML 报告）
task test-cover

# 数据竞争检测
task test-race

# 快速测试
task test-short
```

覆盖率报告输出到 `.artifacts/coverage.html`，目录已在 `.gitignore` 中忽略。

### 测试类型

**单元测试**：

```go
func TestGetUser(t *testing.T) {
    tests := []struct {
        name    string
        userID  int
        want    *User
        wantErr bool
    }{
        {name: "valid user", userID: 1, want: &User{ID: 1}, wantErr: false},
        {name: "invalid user", userID: -1, want: nil, wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := GetUser(tt.userID)
            if (err != nil) != tt.wantErr {
                t.Errorf("GetUser() error = %v, wantErr %v", err, tt.wantErr)
            }
            // 更多断言...
        })
    }
}
```

**基准测试**（性能关键路径）：

```go
func BenchmarkGetUser(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        GetUser(1)
    }
}
```

**示例测试**（公开 API）：

```go
func ExampleGetUser() {
    user, err := GetUser(1)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(user.Name)
    // Output: John Doe
}
```

---

## 代码质量检查

### 提交前检查

**本地检查**（必须通过）：

```bash
# 完整检查
task check

# 提交前快速检查
task pre-commit

# 或分步检查
task fmt        # 格式化代码
task lint       # Lint 检查
task test       # 运行测试
```

### golangci-lint 配置

项目使用 golangci-lint v2，配置文件：`.golangci.yml`

**运行 Lint**：

```bash
# 检查
task lint

# 自动修复
task lint-fix
```

---

## Git 提交规范

### Commit 消息格式

使用全局 Git Commit 模板（`~/.gitmessage`），包含 AI-Meta 信息：

```
[AI-Assisted] 简要描述功能或修复

详细说明：
- 变更 1
- 变更 2

AI-Meta:
- Model: claude-sonnet-4-5
- Session-ID: sess-20251119-xxx
- AI-Generated: 70%
- Human-Modified: 30%
- Related-Spec: FEAT-2025-11-19-001
- Human-Review: 张三, 李四

Reviewed-by: 张三, 李四
Co-Authored-By: Claude <ai@anthropic.com>
```

### 提交步骤

```bash
# 1. 暂存变更
git add .

# 2. 提交（会自动应用模板）
git commit

# 3. 推送到远程
git push origin feature/001-feature-name
```

---

## Merge Request 流程

### 创建 MR

**步骤 1: 使用 MR 模板**

使用 `.gitlab/merge_request_templates/ai-assisted.md` 模板创建 MR。

**步骤 2: 填写 MR 信息**

- **Feature ID**：FEAT-{YYYY-MM-DD}-{序号}
- **关联文档**：Spec、Plan、Tasks 链接
- **AI 协作元数据**：会话 ID、贡献占比、审核人
- **验收确认**：功能性验收、非功能性验收
- **测试证据**：单元测试、集成测试、性能测试、安全扫描

**步骤 3: 检查清单**

- [ ] Phase 0.5 搜索已有方案
- [ ] Spec 包含 10 项强制字段 + 人工审核签字
- [ ] Plan 包含 ADR 记录技术决策
- [ ] 测试覆盖率达标（核心业务 ≥ 95%，整体 ≥ 90%）
- [ ] golangci-lint 检查通过
- [ ] 会话日志已归档

### Code Review 要求

**检查要点**：

- [ ] 代码符合项目原则（constitution.md）
- [ ] 测试覆盖率达标（核心业务 ≥ 95%，整体 ≥ 90%）
- [ ] golangci-lint 检查通过
- [ ] 文档完整（代码注释 + 技术文档）
- [ ] 无明显性能问题
- [ ] 错误处理完善
- [ ] 并发安全（如适用）

**审核人**：

- **架构师**：技术选型、API 设计
- **安全专家**：安全漏洞、敏感数据处理
- **测试经理**：测试覆盖率、测试用例质量

---

## 文档规范

### 文档金标准

所有技术文档必须遵循**文档金标准**（12 个核心原则）：

1. **单一职责**：每个文档只负责一个主题
2. **图表优先**：代码块 ≤ 5 行，超过改用 Mermaid
3. **专业术语**：删除口语化和自问自答
4. **数据驱动**：所有结论有数据支撑
5. **真实可靠**：基于实际代码和数据，不猜测
6. **实质内容**：删除形式主义和表演性内容
7. **时效准确**：只记录最新状态
8. **关联准确**：所有引用和链接有效
9. **体系组织**：符合阅读心智，有清晰编号
10. **代码溯源**：标注仓库、分支、提交
11. **决策留痕**：记录已拒绝方案（ADR）
12. **控制篇幅**：单文件 ≤ 800 行

**参考文档**：
- `.specify/prior-art/01-standards/01-documentation/`

### 文档类型

**技术分析文档**（`.specify/specs/*/analysis/`）：
- 强制遵循文档金标准
- 基于实际代码和数据
- 标注代码基准

**设计文档**（`.specify/specs/*/design/`）：
- 强制遵循文档金标准
- ADR 记录技术决策
- 包含架构图（Mermaid）

**API 文档**（代码注释）：
- 公开 API 必须有中文注释
- 包注释在 doc.go 中
- 示例测试生成文档

---

## 常见问题

### Q1: 如何验证 Go 版本？

```bash
go version  # 应显示 go1.25.4

# 如版本不对，需安装或切换到正确版本
```

### Q2: 如何运行完整检查？

```bash
task check  # Lint + 测试 + 数据竞争检测
```

### Q3: 测试覆盖率不足怎么办？

**要求**：核心业务 ≥ 95%，整体 ≥ 90%

- 查看覆盖率报告：`task test-cover`，打开 `.artifacts/coverage.html`
- 补充测试用例，覆盖未测试的代码路径
- 确保所有公开 API 都有测试
- 核心业务逻辑必须达到 95% 以上

### Q4: golangci-lint 检查失败怎么办？

- 查看错误信息：`task lint`
- 尝试自动修复：`task lint-fix`
- 手动修复剩余问题

### Q5: 如何处理 Breaking Changes？

- 提升 MAJOR 版本号（如 1.x.x → 2.0.0）
- 在 CHANGELOG.md 中明确标注
- 提供迁移指南
- 通过 ADR 记录决策

---

## 参考文档

**项目文档**：
- **项目原则**：`.specify/memory/constitution.md`
- **标准文档索引**：`.specify/prior-art/README.md`
- **文档金标准**：`.specify/prior-art/01-standards/01-documentation/`
- **代码质量标准**：`.specify/prior-art/01-standards/02-code-quality/`
- **测试标准**：`.specify/prior-art/01-standards/03-testing/`

**其他配置**：
- **golangci-lint 配置**：`.golangci.yml`

---
