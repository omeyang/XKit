# 0008 · Mock 置于 `<pkg>mock/` 子包

- **状态**：Accepted
- **范围**：项目级

## 背景

mock 代码通常包含分支繁多但几乎无运行时价值的样板（setters、callback stubs）。若放在主包内，会稀释覆盖率统计，拉低主包真实业务逻辑的覆盖数字，也让 lint 规则（如 errcheck 排除）不得不对 mock 特例放行。

## 决策

- mock 实现放在独立子包 `pkg/xxx/xxxmock/`（或 `internal/xsemaphore/xsemaphoremock/`）。
- mock 子包**不参与**核心覆盖率门禁（`CORE_MIN_COVERAGE=95`）；如接入覆盖率统计，须单独列出。
- 不使用 `go:generate mockgen` 输出到主包目录。

## 备选方案（被拒）

- **mock 与主包同目录**：覆盖率被稀释；需要在 `.golangci.yml` 的 `source:` 模式中特例放行，规则膨胀。
- **mock 放 `testdata/`**：Go 不编译 `testdata/`，mock 无法被同包测试外的其他包复用。

## 影响

- **正向**：主包覆盖率反映真实逻辑；lint 规则保持简洁。
- **代价**：测试需 import 额外子包；目录数量增加。

## 代码引用

- `internal/xsemaphore/xsemaphoremock/`
- `pkg/util/xkeylock`（Locker mock 位置约定）
