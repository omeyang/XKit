# 文档索引

XKit 文档按以下六类组织，均为当前客观状态描述（不记历史演进）。

| # | 类别 | 位置 | 说明 |
|---|---|---|---|
| 01 | 总体目标 | [01-objectives.md](01-objectives.md) | 设计、质量、性能目标 |
| 02 | 约束条件 | [02-constraints.md](02-constraints.md) | 依赖、环境、版本约束 |
| 03 | 业务场景 | [03-scenarios.md](03-scenarios.md) | XKit 的定位与覆盖场景 |
| 04 | 困难与挑战 | [04-challenges.md](04-challenges.md) | 量化指标与硬约束 |
| 05 | 关键决策 | [05-decisions/](05-decisions/00-index.md) | ADR（每决策一文件） |
| 06 | 进度追踪 | [06-progress.md](06-progress.md) | 包稳定性与未完成项 |
| 07 | 约定规范 | [07-conventions/](07-conventions/) | API/命名/贡献 |
| — | 归档 | [_archive/](_archive/) | 已废弃、审查过程记录 |

## 文档原则

- **只记当前状态**：不写"以前 A→后来 B→现在 C"；历史去 `CHANGELOG.md` 或 `_archive/`
- **单一职责**：一份文档一个主题
- **单文件 ≤ 800 行**
- **决策留痕**：每个关键决策单独 ADR，含被拒方案
- **代码溯源**：涉及代码的结论引用包路径
