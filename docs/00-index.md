---
title: 文档索引
tags: [moc, top-level]
---

# 文档索引（顶层 MOC）

XKit 知识库。所有内容是 markdown，用相对路径链接，可在 Obsidian / VS Code / GitHub / 任何 IDE 中直接跳转。

## 主分区

| # | 类别 | 入口 | 说明 |
|---|---|---|---|
| 01 | **关键决策（ADR）** | [01-decisions/](01-decisions/00-index.md) | 8 篇架构决策记录 |
| 02 | 进度追踪 | [02-progress.md](02-progress.md) | 包稳定性矩阵 + 实测覆盖率 |
| 03 | 约定规范 | [03-conventions/](03-conventions/01-api.md) | API 清单 + 贡献流程 |
| 04 | 包详情 | [04-packages/](04-packages/00-index.md) | 每个包一页详细文档 + 子域索引 |
| 05 | 概念笔记 | [05-concepts/](05-concepts/00-index.md) | 跨包概念（错误处理、并发、可观测性等 10 篇） |
| 06 | 设计模式 | [06-patterns/](06-patterns/00-index.md) | 重复出现的解题模板（Cache-Aside / Distributed Lock 等 8 篇） |
| 07 | 术语表 | [07-glossary.md](07-glossary.md) | FG-H / FP / slot / CA/CB/Codex / typed-nil 等 |

> 设计目标 / 约束 / 业务场景 / 质量指标 直接看 [README](../README.md) + [CLAUDE.md](../CLAUDE.md)，避免在多处同步漂移。

## 顶层外部入口

- [README.md](../README.md) - 项目首页
- [CHANGELOG.md](../CHANGELOG.md) - 版本变更
- [/llms.txt](../llms.txt) - AI 索引文件
- `llms-full.txt` - 全文拼接（CI 产物：Actions 工作流附件 + tag 上附加到 release；本地 `task docs-llms` 生成）
- [pkg.go.dev/github.com/omeyang/xkit](https://pkg.go.dev/github.com/omeyang/xkit) - Go API 参考

## 浏览建议

- **首次了解**：[README](../README.md) → [CLAUDE.md](../CLAUDE.md) → [packages MOC](04-packages/00-index.md)
- **想用某个包**：直接进 [04-packages/<域>/](04-packages/00-index.md)
- **想理解某个范式**：[05-concepts/](05-concepts/00-index.md) 或 [06-patterns/](06-patterns/00-index.md)
- **查简写**：[07-glossary.md](07-glossary.md)
- **AI/RAG 喂入**：用 CI 生成的 `llms-full.txt`（见上方"顶层外部入口"段说明）

## 文档原则

- **只记当前状态**：历史演进入 `CHANGELOG.md`
- **单一职责**：一份文档一个主题
- **单文件 ≤ 800 行**
- **决策留痕**：ADR 含被拒方案
- **代码溯源**：涉及代码的结论引用包路径
- **链接优先**：跨主题用相对路径链接，不复制内容
