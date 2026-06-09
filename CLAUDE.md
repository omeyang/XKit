# CLAUDE.md

XKit 协作入口。AI 助手与新协作者读本文件先于其他文档。

## 任务入口

| 任务 | 命令 |
|---|---|
| 完整本地检查（push 前必跑） | `task pre-push` |
| Lint | `task lint` |
| 测试 | `task test` / `task test-cover` / `task test-race` |
| 基准测试 | `task bench` |
| 集成测试（需中间件） | `go test -tags=integration ./pkg/...` |

`task pre-push` 包含 fmt-check / lint-ci / vulncheck / test-short——与 `.github/workflows/ci.yml` 同源。**不要把静态扫描交给 CI 发现。**

## 项目结构

- 包：`pkg/<domain>/<pkg>/`（38 个；分类见 [`docs/00-index.md`](docs/00-index.md)）
- 内部：`internal/`（仅项目内部使用）
- CLI：`cmd/xdbgctl`

主要导航：

- 公开 API 清单：[`docs/03-conventions/01-api.md`](docs/03-conventions/01-api.md)
- 稳定性矩阵 + 覆盖率：[`docs/02-progress.md`](docs/02-progress.md)
- 关键决策（ADR，8 篇）：[`docs/01-decisions/`](docs/01-decisions/00-index.md)
- 跨包概念：[`docs/05-concepts/`](docs/05-concepts/00-index.md)
- 设计模式：[`docs/06-patterns/`](docs/06-patterns/00-index.md)
- 术语：[`docs/07-glossary.md`](docs/07-glossary.md)

## 硬约束

- Go 版本：**1.25.10**（固定，`task verify` 校验）；1.23 兼容分支 `develop-1.23-release`
- Lint：`golangci-lint v2.11.4`，`.golangci.yml` 严格（`errcheck.check-blank: true` / `check-type-assertions: true` / `funlen.max=70` / `gocyclo<=10`）
- 错误：跨包用 `%w`；抽象边界用 `%v`（须有 `// 设计决策:` 注释，ADR 0004）
- 构造函数返 `error`，不 `panic`（ADR 0001）
- 接口由使用方定义（ADR 0002）
- 函数选项模式（ADR 0003）
- mock 放 `<pkg>mock/` 子包，不拖低主包覆盖率（ADR 0008）
- 覆盖率目标：核心包 ≥ 95%，整体 ≥ 90%
- 中文文档、英文标识符（ADR 0006）

## 文档纪律

- **只记当前状态**——不写"以前 A → 后来 B → 现在 C"
- 历史演进入 `CHANGELOG.md`
- `.githooks/docs-ledger-check.sh` 强制（pre-commit 在改 docs/README 时自动跑）
- 单文件 ≤ 800 行

## 提交规范

- **禁止** `Co-Authored-By` / `Claude` / `Generated with` 等 AI 署名（`.githooks/commit-msg` 强制）
- 启用 hooks：`task install-hooks`（设 `core.hooksPath .githooks`）

## 对抗审查

- 配置：[`.adversarial-review.yaml`](.adversarial-review.yaml)
- 工具位置（按优先级查找）：
  1. `$ADVERSARIAL_REVIEW_HOME`（环境变量显式指定）
  2. `~/code/ai/github.com/omeyang/ai-toolkit/workflows/adversarial-review/`（本机默认）
  3. `$GOPATH/src/github.com/omeyang/ai-toolkit/workflows/adversarial-review/`（兜底搜索）
- 触发：pre-commit 增量审查 + cron 定时全包

## 运行时环境

Linux/Rocky 10；Go 1.25.10；`task`（go-task）；包管理 `dnf5`。
