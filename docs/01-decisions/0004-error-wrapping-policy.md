# 0004 · 错误包装：`%w` 保链，抽象边界用 `%v`

- **状态**：Accepted
- **范围**：项目级

## 背景

`fmt.Errorf` 的 `%w` 保留错误链，允许 `errors.Is/As` 穿透；`%v` 只保留文本。跨抽象边界（例如用户注入的 ID 生成器内部错误）若用 `%w`，会让调用方窥探到不属于公开契约的具体错误类型，形成隐式耦合。

## 决策

- **默认使用 `%w`**：同一抽象层、同一包内部、对外公开的 sentinel 错误链，均用 `%w`。
- **抽象边界用 `%v`**：用户注入的回调/生成器/钩子抛出的错误，若对外应只以"我方 sentinel"形式暴露，内部原因用 `%v` 拼接。
- 每处 `%v` 须有 `// 设计决策:` 注释解释边界。
- 双 wrap 语法 `%w: %w` 用于两个同层 sentinel 同时保留（如 `fmt.Errorf("%w: %w", ErrA, cause)`）。

## 备选方案（被拒）

- **全量 `%w`**：泄漏内部错误类型，破坏封装。
- **全量 `%v`**：调用方无法 `errors.Is(err, ErrX)`，丧失 Go 1.13+ 错误链的核心价值。

## 影响

- **正向**：抽象稳定，`errors.Is/As` 契约清晰。
- **代价**：每次新增错误包装点需判断边界归属；评审成本上升。

## 代码引用

- `%w` 链：`pkg/util/xnet.IPSetFromRanges`（`ErrInvalidRange` 包装上游）
- `%v` 边界：`internal/xsemaphore.ErrIDGenerationFailed`（ID 生成器内部错误）
- 双 wrap：`internal/xid.Init`（配置错误同时链 `ErrInvalidConfig` 与 cause）
