# 01-Go 依赖管理标准

**代码基准**：`git.sangfor.com/apex/asm/xkit` @ `master` (`42a8fe0`, 2025-11-19)

## 1. 目标与范围

- 定义 XKit 仓库在 Go 依赖管理方面的统一约束。
- 覆盖 `go.mod` module 路径、外部依赖引入、版本升级和安全检查。
- 适用于：
  - `.specify/specs/*/design/` 中涉及依赖的设计决策；
  - 所有修改 `go.mod` / `go.sum` 的变更。

## 2. Module 路径

- `go.mod` 中的 module 路径 **必须** 与 Git 仓库路径保持一致：

```go
module git.sangfor.com/apex/asm/xkit
```

- 禁止在其他项目中依赖非仓库同名路径（例如 `sangfor.com/xdr/xkit`）。
- 任何偏离上述路径的尝试都应在对应 Feature 的 ADR 中记录并明确拒绝。

## 3. 依赖引入原则

- 优先使用标准库；只有在明确收益大于维护成本时才引入第三方依赖。
- 新增第三方依赖 **必须**：
  - 在对应 Feature 的设计文档中说明用途和替代方案；
  - 在 ADR 中记录决策；
  - 评估维护状态和安全风险。
- 依赖版本管理遵循语义化版本规则，避免使用未标记版本的临时 commit。

## 4. 版本与升级策略

- 常规升级：
  - 使用 `go get -u` 升级次要版本或补丁版本；
  - 确保测试和 lint 全部通过后再合入。
- 重大升级（可能引入 Breaking Changes）必须：
  - 在 ADR 中记录升级动机、影响范围和回滚方案；
  - 在 `CHANGELOG.md` 中标注对外可见的行为变化。
- 禁止在不同 Feature 中同时对同一个依赖做不协调的版本调整。

## 5. 检查方法

- 校验 module 路径：

```bash
grep '^module ' go.mod
```

- 查看依赖列表和版本：

```bash
go list -m all
```

- 变更评审时，审查 `go.mod` / `go.sum` 的 diff，确保只包含预期依赖调整。

## 6. 与其他标准的关系

- 依赖管理决策应遵循：
  - `.specify/prior-art/01-standards/01-documentation/` 中的文档金标准（溯源和数据支撑要求）；
  - `.specify/prior-art/01-standards/02-code-quality/` 中的设计原则；
  - `.specify/prior-art/01-standards/03-testing/` 中的测试覆盖率要求。

