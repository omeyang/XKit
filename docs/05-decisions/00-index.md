# 05 · 关键决策记录（ADR）

每个决策一份 ADR，按创建顺序编号（`NNNN-kebab-case-title.md`）。格式见 [template.md](template.md)。

## 原则

- **一决策一文件**：单个 ADR 聚焦单个决策。
- **状态**：`Proposed` / `Accepted` / `Superseded by NNNN` / `Deprecated`。被替代的 ADR 保留文件，在标题行标注 Superseded。
- **必须包含**：背景 / 决策 / 被拒方案 / 影响。
- **引用代码**：决策对应的代码位置与 commit（落地时补）。

## 跨项目基础决策

| # | 标题 | 状态 |
|---|---|---|
| [0001](0001-constructor-returns-error-not-panic.md) | 构造器返回 error 而非 panic | Accepted |
| [0002](0002-interface-defined-by-consumer.md) | 接口由使用方定义而非提供方 | Accepted |
| [0003](0003-functional-options-for-config.md) | 使用函数选项模式承载配置 | Accepted |
| [0004](0004-error-wrapping-policy.md) | 错误包装：`%w` 保链，抽象边界用 `%v` | Accepted |
| [0005](0005-slog-as-sole-log-backend.md) | 采用 `log/slog` 作为唯一日志后端 | Accepted |
| [0006](0006-chinese-docs-english-identifiers.md) | 中文文档注释 + 英文标识符 | Accepted |
| [0007](0007-util-packages-no-builtin-observability.md) | 通用工具包不内置可观测性 | Accepted |
| [0008](0008-mock-in-subpackage.md) | Mock 置于 `<pkg>mock/` 子包 | Accepted |

## 待从 `MEMORY.md` 提炼的候选 ADR

以下决策已在 `memory/` 与各包 doc 中以"设计决策:"形式散落，待逐条提升为 ADR：

- `xsemaphore`: Release/Extend 不做 duration histogram；WarmupScripts 顺序加载；ErrIDGenerationFailed 用 `%v`；releaseCommon 用 Load 而非 Swap；metrics prefix `xsemaphore.*`
- `xid`: NewGenerator 不做全局 machine ID 冲突检测；NewWithRetry 对时钟回拨 sleep 不可中断
- `xlru`: OnEvicted 在上游 mutex 内运行（死锁约束）；Close TOCTOU 窗口可接受；TTL=0 透传上游 10 年哨兵
- `xmac`: iter 四函数结构重复保留（Go 类型系统 + 性能）；CollectN 上限 `1<<20`
- `xnet`: `WireRange.String()` 单 IP 与单点段歧义；`IsBroadcast` 优先级高于 `IsReserved`
- `xpool`: Submit 非阻塞为设计；无内建 observer hooks；handler 签名 `func(T)` 故意不带 ctx
- `xkafka`: `RetryTopic=""` 语义为 requeue 到原 topic，不保证分区顺序
- `xlimit`: 空规则 fail-open；多规则非原子；中间件 fail-open
- `xtrace`: 出站注入归 OTel propagator；parseTraceparent 拒绝 v00 非法 flags
- `xmetrics`: typed-nil provider 过滤；tracer.Start 返回 nil ctx 兜底；histogram buckets first-registration-wins

每条对应一个新 ADR 文件，不要合并。
