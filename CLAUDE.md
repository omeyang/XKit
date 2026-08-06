# CLAUDE.md

XKit 协作入口。AI 助手与新协作者读本文件先于其他文档。

## 任务入口

| 任务 | 命令 |
|---|---|
| 完整本地检查（push 前必跑） | `task pre-push` |
| Lint（与 CI 同版本同超时，以此为准） | `task lint-ci` |
| Lint（本地快速迭代，版本随 PATH 不固定） | `task lint` |
| 测试 | `task test` / `task test-cover` / `task test-race` |
| 基准测试 | `task bench` |
| 集成测试（需中间件） | `go test -tags=integration ./pkg/...` |

`task pre-push` 包含 check-toolchain / fmt-check / lint-ci / mod-check / build / cross-check / test-short / docs-ledger-check，覆盖 CI 的 **Go 侧**静态检查。仅 CI 跑的部分：actionlint（工作流配置）、`-race` 全量测试、覆盖率上报、`llms-full.txt` 生成。**不要把 Go 侧静态扫描交给 CI 发现。**

不含 govulncheck：本分支的 Go 1.23 已停止安全维护，扫描恒为红，见 `CHANGELOG.md`。

## 项目结构

- 包：`pkg/<domain>/<pkg>/`（分类见 [`docs/00-index.md`](docs/00-index.md)；包数与稳定性以 [`docs/02-progress.md`](docs/02-progress.md) 为单一事实源）
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

- Go 版本：**1.23.x**（`task check-toolchain` 校验）；本分支为 1.23 兼容版本，主线见 `main`（Go 1.24.6）
- 与 `main` 的可观察行为差异：[`docs/08-1.23-branch-notes.md`](docs/08-1.23-branch-notes.md)
- go.mod 的 `go` 指令为**最低语言版本**（由依赖 MVS 决定），不是工具链锁；不写 `toolchain` 指令——内网 `GOTOOLCHAIN=local` + `GOSUMDB=off`，任何工具链自动下载必失败
- 经 `go run` / `go install` 从源码构建的工具（golangci-lint / gocyclo / actionlint），其 go.mod 的 `go` 指令必须 **≤ 1.23**；升级前先查该行，版本在 `Taskfile.yml` 与 `ci.yml` 中固定
- 不使用 Go 1.24+ 专有 API（如 `testing.B.Loop`、`sync.WaitGroup.Go`、`testing/synctest`）
- Lint：`golangci-lint v2.3.1`，`.golangci.yml` 严格（`errcheck.check-blank: true` / `check-type-assertions: true` / `funlen.max=70` / `gocyclo<=10`）
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
- 工具位置：`${AI_TOOLKIT_HOME:-~/code/ai/github.com/omeyang/ai-toolkit}/workflows/adversarial-review/`
  （只认 `AI_TOOLKIT_HOME` 这一个环境变量；找不到时 pre-commit 打印告警并放行，不阻断提交）
- 触发：pre-commit 增量审查 + cron 定时全包
- 钩子：全部经 `.githooks/`，由 `task install-hooks` 一次装好。`core.hooksPath` 只能指一处，
  勿把对抗审查另装到 `.git/hooks/`——两套互斥，会让 `.githooks/` 整体失效。

## 运行时环境

Linux/Rocky 10；Go 1.23.x；`task`（go-task）；包管理 `dnf5`。
