# Adversarial Review Log

## 2026-04-14 slot=0 TARGET=xelection
- 4 份报告原始发现：Claude 攻=5 (1H/4M) 守=1 (1M) / Codex A=日志截断无结论 B=日志截断无结论
- 高置信必修=1 单源采纳=0 舍弃=4
- 高置信必修：Resign 错误链丢失（Claude 攻 BUG-1 H）— `etcd.go:256` `err != nil && resignErr == nil` 静默吞掉 session.Close 错误；改 errors.Join 合并。
- 研判舍弃：
  - observeDone 无 ctx 超时（两 Claude 提）— 设计决策：observeCancel 已保证快速退出，ctx-select 会引入 goroutine 泄漏风险，不采纳
  - Campaign errors.Join 外嵌 fmt.Errorf 语义（BUG-2）— 风格差异，非缺陷
  - observe/Resign 原子竞态（BUG-3）— 最终状态一致，非真 bug
  - non-etcd session 分支错误叠加（BUG-4）— 不可达路径（仅测试注入）
- 修复：commit 81725df；`task pre-push` 绿；已推送 main

## 2026-04-14 slot=0 TARGET=xclickhouse
- 原始发现：Claude 攻=3 守=6 / Codex A=3 B=2
- 交叉对抗：Codex 攻 Claude → 输出被工具日志截断未生成表格（手动裁决 9 条全为 b/文档化设计）；Claude 攻 Codex → a=4 c=1 b=0
- 合议：必修=3 存疑=0 舍弃=12
- 必修项：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |---|---|---|---|---|---|
  | CodexA1 | FG-H | clickhouse.go:172 | typed-nil driver.Conn 绕过 client==nil 检查 | 必修 | 1+CC(a) |
  | CodexA2 | FG-H | wrapper.go:579 | nil row 传入 AppendStruct 触发 reflect panic | 必修 | 1+CC(a) |
  | CodexA3 | FG-M | clickhouse.go:177 | opts 含 nil Option 时直接调用 nil 函数 panic | 必修 | 1+CC(a) |
- 舍弃要点：
  - Claude CA1 `errors.Join(nil,x)` — 语义等价 x，errors.Join 过滤 nil
  - Claude CA2/CA3/CB1/CB2/CB5 — 已有文档化设计决策或公共 API 契约
  - Claude CB3/CB6 — ValidatePagination 已保证 offset/pageSize 正整数+上界
  - Claude CB4 — appendedCount 冗余判断为防御性，语义等价
  - Codex B1 — insertBatches 跨批次部分成功属 BatchResult 文档契约
  - Codex B2 — Close CAS 提前置位是标准 Go 惯例（conn.Close 失败不应重试）
- 修复：commit d815f08；`task pre-push` 绿；已推送 main
