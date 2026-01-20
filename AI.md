# XKit - AI 协作说明

**开发方法**: Spec-Driven Development（基于 GitHub Spec Kit）  
**主力工具**: Claude Code（`/speckit.*` 斜杠命令）  
**协作规范**: AI 协作规范 v1.2

## 快速开始

Phase 0: `/speckit.constitution` — 建立/维护项目原则  
Phase 0.5: （暂不自动化）手动先搜索现有方案  
Phase 1: `/speckit.specify` — 创建 Feature 规格（`spec.md`，含 10 项强制字段）  
Phase 2: `/speckit.plan` — 创建技术计划（`plan.md` + `data-model.md` 等）  
Phase 3: `/speckit.tasks` — 任务拆解（`tasks.md`，可多人协作）  
Phase 4: `/speckit.implement` — 按任务实现与提交  
Phase 5: `/speckit.archive-session`（后续补充）— 会话归档

## 目录约定

- `.specify/memory/constitution.md`：项目原则（本仓库已提供基础版本）
- `.specify/specs/{feature-id-short-name}/`：单个 Feature 的规格与计划
  - `spec.md`：需求规格（WHAT & WHY，10 项强制字段）
  - `plan.md`：技术计划（HOW，含溯源矩阵与复用清单）
  - `tasks.md`：任务拆解与分工
  - 其它：`data-model.md`、`contracts/`、`research.md`、`quickstart.md`、`ai-sessions/`
- `.specify/prior-art/`：与本项目强相关的先验知识（按需扩展，避免堆砌）
- `.claude/CLAUDE.md`：AI 使用说明与工作流详情

所有 Feature 目录建议直接使用 Feature ID 命名，例如：

- `.specify/specs/feature-1030039-refactor/`

## 工具收敛

当前仓库仅面向 Claude Code 与本地 CLI 使用：

- 不预创建 Gemini / Copilot / Cursor 等目录
- 如团队后续实际使用，再按需补充对应配置文件

## 参考

- 全局规范：`/root/code/ai/documents/tools-and-configs/sangfor/internal-standards/2. AI协作规范v1.2.md`
- Lint 与 Go 工具链规范：同上文档中的 Go 相关章节
