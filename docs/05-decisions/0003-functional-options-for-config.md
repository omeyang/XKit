# 0003 · 使用函数选项模式承载配置

- **状态**：Accepted
- **范围**：项目级

## 背景

配置项随时间会增加。构造器直接收 `Config` 结构体会导致：新增字段即破坏零值调用者；必填 vs 可选字段不能在类型层表达；默认值散落。

## 决策

公开构造器参数分两类：

1. **必填**：作为显式位置参数（如 `client *redis.Client`、`capacity int`）。
2. **可选**：通过 `...Option` 变参 + `WithXxx` 函数注入；选项结构体私有（`options struct`）；提供 `(o *options) validate() error` 统一校验。

命名：包内选项类型 `XxxOption`（接口或 func 类型），构造函数 `WithXxx`。

## 备选方案（被拒）

- **公开 Config struct**：新增必填字段即破坏兼容；零值语义易歧义。
- **Builder 模式**：链式调用在 Go 中不惯用，且失败延迟到 `Build()` 调用，对参数来源复杂的场景无价值。

## 影响

- **正向**：API 兼容性好；默认值集中；选项可组合/测试。
- **代价**：每个选项一个 `WithXxx` 函数；大量选项时样板代码增加。

## 代码引用

- `pkg/util/xpool`、`internal/xsemaphore`、`pkg/observability/xlog` 均采用此模式
- 校验入口：`options.validate()`（见 `internal/xsemaphore/options.go`）
