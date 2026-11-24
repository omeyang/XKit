# 运行 XKit 测试

在仓库根目录运行 `task test`。运行环境为 Go 1.24.6 和 Python 3。每个测试只有一份源码。

| 目录 | 内容 |
|---|---|
| `unit/<原包路径>/` | 单元测试、示例与包内测试辅助函数。混合用例文件随所属测试套件存放。 |
| `bench/<原包路径>/` | 独立的基准测试文件 |
| `fuzz/<原包路径>/` | 模糊测试与种子用例 |
| `integration/<原包路径>/` | 带 integration 构建标签的中间件测试 |
| `e2e/` | 跨包端到端测试 |

`tests/run` 通过 Go 的 `-overlay` 将前四类测试附加到对应的业务包，同时将收纳目录从本次包枚举中隐藏。这样包内测试仍可使用私有成员，覆盖率也计在原包。e2e 是独立的 Go 测试包。直接在仓库根运行裸 `go test` 不会加载这个映射；使用以下入口。

```sh
task test
task test-cover
task test-race
tests/run go test -run TestSafeJoin ./pkg/util/xfile
tests/run go test -bench=. -run='^$' ./pkg/storage/xetcd
tests/run go test -tags=integration ./pkg/storage/...
tests/run go test -tags=e2e ./tests/e2e
```

新增测试时保留原 Go 包名、平台文件名后缀和构建标签。同一被测包中的测试文件名须唯一。运行器每次从目录生成映射，各次调用使用各自的临时目录。Go 的测试选择、并行度和覆盖率参数原样传递。

`task lint-ci` 会在临时目录内按原包关系链接源码，供直接打开源文件的 lint 工具读取。`task size-check` 检查真实源码路径，包括未启用构建标签的文件。函数与匿名函数从声明到右花括号按物理行计数，上限 150 行。Go 文件目标 800 行，801–820 行会提示，超过 820 行检查失败。
