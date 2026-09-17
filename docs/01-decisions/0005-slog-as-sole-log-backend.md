# 0005 · 采用 `log/slog` 作为唯一日志后端

- **状态**：Accepted
- **范围**：项目级

## 背景

Go 1.21+ 标准库提供 `log/slog`，结构化日志、Handler 抽象、Level 控制均为官方契约。引入第三方日志库（zap/zerolog）会增加依赖、绑定特定 API、与 OTel log bridge 衔接成本高。

## 决策

XKit 统一以 `log/slog` 为日志后端：

- `pkg/observability/xlog` 提供 Builder、上下文增强、全局 logger。
- 不导出自建 Logger 接口；需要可替换后端时，交由 `slog.Handler` 注入。
- 库内部日志一律走 `slog.Default()` 或包内传入的 `*slog.Logger`，禁止 `fmt.Print*`、`log.Print*`。

## 备选方案（被拒）

- **zap**：性能优秀但 API 与标准库不兼容，迁移成本高；零分配优势在 XKit 多数路径不显著。
- **自建 Logger 接口**：每个消费者 wrap 一层，丧失 `slog.Handler` 生态。

## 影响

- **正向**：零额外依赖；与 OTel log bridge、`slog.LogValuer` 生态天然兼容。
- **代价**：`slog` 热路径有分配（已通过 `LogAttrs` + 预构造 Attr 缓解，见 `pkg/context/xctx`）。

## 代码引用

- `pkg/observability/xlog`
- `pkg/context/xctx.LogAttrs`（零分配空 context 路径）
- `pkg/util/xpool.safeHandle` 用 `slog.LogAttrs` 记录 panic
