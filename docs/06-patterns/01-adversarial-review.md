---
title: 对抗审查模式
tags: [pattern, review, ai]
---

# 对抗审查（Adversarial Review）

## 问题

单个 AI 审查代码有盲区与偏见。如何系统性提高发现率？

## 解法：多路 AI 互审 + 交叉裁决

XKit 的对抗审查流程使用 4 路 AI 同时审 + 1 路交叉裁决：

| 角色 | 含义 |
|---|---|
| CA | Claude A：常规审查（找问题）|
| CB | Claude B：换角度审（同一代码不同切入点）|
| Codex A | OpenAI Codex 第一组 |
| Codex B | OpenAI Codex 第二组 |
| CC | Claude 交叉裁决：判定其他 4 路发现的真伪（a=真问题 / b=FP / c=不确定）|

## 流程

```
某包代码
   ├── CA  → 发现列表 (FG-H/M/L 分级)
   ├── CB  → 发现列表
   ├── Codex A → 发现列表
   └── Codex B → 发现列表
              ↓
        交叉对抗审：每路用对方发现攻自己
              ↓
        CC 裁决：每条发现判 a/b/c
              ↓
       人工/MEMORY 记录真问题修复
```

## 发现的分级（FG）

| 级别 | 含义 |
|---|---|
| FG-H | 高严重（并发 race / NPE / 数据丢失）|
| FG-M | 中严重（错误处理瑕疵、文档不准）|
| FG-L | 低严重（命名、格式）|
| FG-S | 安全或样式问题（可独立汇总）|

## 真问题 vs FP（False Positive）

约 80-90% 发现是 FP。常见 FP 模式：

- "该字段可能并发读写" → 实际有 mutex 保护
- "TOCTOU 窗口" → 文档化为可接受设计取舍
- "%v 应改 %w" → 抽象边界设计决策
- "缺 nil 检查" → Go 标准库也不防 typed-nil
- "可能 goroutine 泄漏" → 实际由 WaitGroup 保证退出

XKit MEMORY 大量记录这些 FP，避免重复发现。

## 调度

XKit 的对抗审查是**自动化定时任务**：

- cron 触发（凌晨 0-7 点，避开工作时段）
- 固定 SLOT→包 映射（15 个包轮转）
- pre-commit 增量审查（仅审改动）
- 工具仓在 `~/code/ai/github.com/omeyang/ai-toolkit/workflows/adversarial-review/`
- XKit 自身的 `.adversarial-review.yaml` 入仓

## 历史成果

- 30+ 包覆盖
- ~145 条 docs(review) commit（已 squash 到 v0.1.0）
- 真问题修复（节选）：xsemaphore typed-nil、xtrace propagator nil 守卫、xkafka ctx 取消前置检查、xhealth statusListener recover、xelection Resign nil ctx、xpool safeHandle 嵌套 recover、xauth nil ctx → ErrNilContext 等

## 相关

- 配置：[`.adversarial-review.yaml`](../../.adversarial-review.yaml)
