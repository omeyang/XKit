# 归档区

本目录存放：

- **审查过程记录**：Codex/人工审查的往来讨论、修复清单
- **废弃决策**：已被新 ADR 替代但保留的文件
- **研究/探索笔记**：未落地为正式决策的调研

归档文件**不受"禁止记账"规则约束**——这里就是记账的地方。

## 组织

- 审查过程：`review-<package>-<YYYY-MM-DD>.md`
- 废弃决策：从 `docs/05-decisions/` 移入，保持原文件名，状态字段改为 `Deprecated` / `Superseded by NNNN`
- 研究笔记：`research-<topic>.md`

## 引用

若正式文档需要引用归档内容，使用相对路径（例 `../_archive/review-xsemaphore-2026-04-10.md`）。
