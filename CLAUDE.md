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
| 集成测试（需中间件） | `tests/run go test -tags=integration ./pkg/...` |

`task pre-push` 包含 check-toolchain / fmt-check / size-check / lint-ci / mod-check / build / cross-check / test-short / docs-ledger-check，覆盖 CI 的 **Go 侧**静态检查。仅 CI 跑的部分：actionlint（工作流配置）、`-race` 全量测试、覆盖率上报、`llms-full.txt` 生成。**不要把 Go 侧静态扫描交给 CI 发现。**

项目已接受固定 Go 1.24.6 下的 govulncheck 扫描结果，保留按需扫描，发布门禁沿用此例外。

## 项目结构

- 包：`pkg/<domain>/<pkg>/`（分类见 [`docs/00-index.md`](docs/00-index.md)；包数与稳定性以 [`docs/02-progress.md`](docs/02-progress.md) 为单一事实源）
- 测试：[tests/README.md](tests/README.md)。测试源码集中于 `tests/`，包级筛选使用 `tests/run go test`。
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

- Go 版本：**1.24.6**（内网流水线构建底座固定版本，`task check-toolchain` 精确校验）；仅维护 `main` 分支
- `go.mod` 声明最低 Go 版本为 `1.24.6`，`task check-toolchain` 校验实际使用的工具链。内网流水线设置 `GOTOOLCHAIN=local`，使用预装的 Go 1.24.6。
- 经 `go run` / `go install` 从源码构建的工具（golangci-lint / gocyclo / actionlint），其 go.mod 的 `go` 指令必须 **≤ 1.24.6**；升级前先查该行，版本在 `Taskfile.yml` 与 `ci.yml` 中固定
- 不使用 Go 1.25+ 专有 API（如 `sync.WaitGroup.Go`、`testing/synctest`、`T.Output`）
- Lint：`golangci-lint v2.8.0`，`.golangci.yml` 严格（`errcheck.check-blank: true` / `check-type-assertions: true` / `funlen.max=70` / `gocyclo<=10`）
- 错误：跨包用 `%w`；抽象边界用 `%v`（须有 `// 设计决策:` 注释，ADR 0004）
- 构造函数返 `error`，不 `panic`（ADR 0001）
- 接口由使用方定义（ADR 0002）
- 函数选项模式（ADR 0003）
- mock 放 `<pkg>mock/` 子包，不拖低主包覆盖率（ADR 0008）
- 覆盖率目标：核心包 ≥ 95%，整体 ≥ 90%
- 业务与测试函数均不超过 150 行。Go 文件目标 800 行，801–820 行提示，超过 820 行门禁失败。
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
- 触发：启用 Git 钩子且外部工具可用时，pre-commit 执行增量审查
- 钩子：全部经 `.githooks/`，由 `task install-hooks` 一次装好。`core.hooksPath` 只能指一处，
  勿把对抗审查另装到 `.git/hooks/`——两套互斥，会让 `.githooks/` 整体失效。

## 运行时环境

Linux/Rocky 10；Go 1.24.6；`task`（go-task）；包管理 `dnf5`。
