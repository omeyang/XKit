---
description: Create or update a feature spec (spec.md) following AI 协作规范 v1.2.
handoffs:
  - label: Build Technical Plan
    agent: speckit.plan
    prompt: Create a plan for the approved spec.md
  - label: Clarify Spec Requirements
    agent: speckit.clarify
    prompt: Clarify open questions in the spec
    send: true
---

## User Input

```text
$ARGUMENTS
```

## Workflow

1. **解析需求说明**
   - `$ARGUMENTS` 即为用户输入的 Feature 自然语言描述（WHAT & WHY）
   - 如内容为空，直接报错：`No feature description provided`

2. **生成短名并创建编号目录**
   - 依据描述生成 2–3 个英文单词的短名：如 `xkit-init`、`logging` 等
   - 使用 `.specify/scripts/bash/create-new-feature.sh --json`：
     - 自动选择下一个可用编号（001, 002, …）
     - 创建分支 `NNN-short-name`
     - 创建目录 `.specify/specs/NNN-short-name/spec.md`
   - 解析脚本输出中的 `BRANCH_NAME` 与 `SPEC_FILE`

3. **载入模板与上下文**
   - 读取 `.specify/templates/spec-template.md`
   - 读取 `.specify/memory/constitution.md` 了解项目原则
   - 按模板中的 10 个章节（元数据、溯源、差异化、User Stories、验收、RAC、ADR、功能需求、反幻觉、安全与审核）依次填充

4. **对齐 AI 协作规范 v1.2 要求**
   - **元数据**：填充 `Feature ID`（如 `feature-1030039-refactor-xkit`）、负责人、AI 模型、状态等
   - **需求溯源**：要求至少给出 1 个上游文档 / 需求来源；若未知，用 `[NEEDS CLARIFICATION: ...]` 标记
   - **差异化与创新点**：补一张对比表 + 至少 3 条独特价值
   - **User Stories**：按用户视角 + 验收条件书写（可引用用户原话）
   - **验收标准**：用 SMART 方式给出功能与非功能指标
   - **RAC 矩阵**：至少列出 1 条风险与 1 条假设
   - **功能需求**：用 `FR-1/2/...` 形式列出可测试的需求
   - **反幻觉验证**：对关键技术决策和依赖库给出"AI 建议 vs 人工确认"的表格

5. **写回 spec.md**
   - 在 `SPEC_FILE` 中写入完整 Markdown 内容，保持结构与 `.specify/templates/spec-template.md` 一致
   - 禁止输出与本 Feature 无关的通用教学/表演性内容

6. **输出总结**
   - 报告：`BRANCH_NAME`、`SPEC_FILE` 路径、主要风险与待澄清项数量
   - 提示下一步可使用 `/speckit.plan` 继续 Phase 2。