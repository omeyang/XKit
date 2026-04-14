# 0001 · 构造器返回 error 而非 panic

- **状态**：Accepted
- **范围**：项目级

## 背景

XKit 的消费方是业务服务，构造期失败（参数非法、依赖客户端初始化失败）需要由业务代码决定处理策略（降级、重试、fail-fast）。若库在构造期 panic，会绕过业务的错误处理与可观测性管道。

## 决策

所有公开构造器（`New*`、`Build*`、`Init*`）在输入非法或依赖初始化失败时返回 `error`，**不得 panic**。

例外：纯值类型的 `MustXxx` 可基于已校验常量使用（如 `xmac.MustParse`），但必须有对应的 `Parse` 错误版本。

## 备选方案（被拒）

- **panic + recover**：调用方记忘 recover 会炸进程；且 panic 栈无结构化字段，难以纳入监控。
- **sentinel 零值 + 延迟失败**：首次调用才报错会污染调用链，违反"最早失败"原则。

## 影响

- **正向**：构造期错误可统一纳入业务日志/监控；单元测试用 `wantErr` 即可覆盖非法路径。
- **代价**：构造器签名多一个返回值；零值 pattern 的简化被牺牲。

## 代码引用

- 全包普遍遵循；示例：`pkg/observability/xlog`、`pkg/storage/xcache`、`internal/xid.Init`
