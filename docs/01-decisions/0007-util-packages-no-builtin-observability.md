# 0007 · 通用工具包不内置可观测性

- **状态**：Accepted
- **范围**：项目级（`pkg/util/*`）

## 背景

`xpool`、`xlru`、`xkeylock`、`xnet` 等工具包被高频调用（热路径），且部署环境的可观测性方案由业务选型。若库内置 metrics/tracing，会带来运行时依赖与耦合。

## 决策

`pkg/util/*` 包**不内置**任何 metrics、tracing、structured log 埋点：

- 不 import `xmetrics` / OTel / `xlog`。
- 不暴露 observer hook、callback 埋点（`xpool.WithObserver` 类 API 不提供）。
- 若有错误路径需要日志，使用 `slog.Default()` 简短记录（如 `xpool.safeHandle` 的 panic 日志），不做结构化指标。

可观测性由调用方在业务层包一层装饰器（`Wrap`/`Decorator`）实现。

## 备选方案（被拒）

- **内置 OTel 埋点**：热路径引入额外分配与依赖；默认关闭也要付出接口表面积代价。
- **暴露 hook 接口**：每个 hook 点都是潜在 panic/死锁源；工具包契约应该收敛。

## 影响

- **正向**：工具包依赖最小；热路径零额外开销。
- **代价**：业务需自行包装监控层。

## 代码引用

- `pkg/util/xpool/doc.go`（设计决策：无内置 observability）
- `pkg/util/xlru`（Contains/Peek 路径无埋点）
- 反例：`pkg/resilience/*`、`pkg/mq/*`、`internal/xsemaphore` **有**内置 OTel 埋点（属于"需要可观测性契约"的基础设施层）
