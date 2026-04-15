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

## 2026-04-16 slot=0 TARGET=xcache
- 原始发现：Claude攻=3 守=4 / Codex A=0(输出为工具日志无表格) B=0(同)
- 交叉对抗：Codex攻Claude → 挂起(stdin管道问题)未产出；Claude攻Codex → N/A(Codex无发现)
- 合议：必修=0 存疑=0 舍弃=7
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |---|---|---|---|---|---|
  | CA-1 | FG-H | loader.go:182 | 双%w据称仅第一个生效 | 舍弃 | 1(CA) |
  | CA-2 | FG-H | loader_impl.go:382 | 双%w错误链丧失 | 舍弃 | 1(CA) |
  | CA-3 | FG-M | loader_impl.go:84 | contextDetached nil防御 | 舍弃 | 1(CA) |
  | CB-1 | FG-M | loader_impl.go:272 | WithCancel泄漏 | 舍弃 | 1(CB) |
  | CB-2 | FG-M | loader_impl.go:104 | 语义混淆 | 舍弃 | 1(CB) |
  | CB-3 | FG-H | loader_impl.go:120 | 两函数不一致 | 舍弃 | 1(CB) |
  | CB-4 | FG-H | loader_impl.go:448 | ctx文档不明 | 舍弃 | 1(CB) |
- 舍弃要点：
  - CA-1/CA-2: Go 1.20+ 支持多 %w，`errors.Is` 两个 sentinel 均生效；项目惯例（xid 包同模式）
  - CA-3: 公共入口 Load/LoadHash 已验 nil ctx (L169)，contextDetached 仅内部调用
  - CB-1: context.WithCancel 不创建 goroutine，sfCancel defer 正确释放
  - CB-2/CB-3: 两函数目的不同（detach vs no-detach），timeout==0 语义各自文档化(L94-97, L115-128)
  - CB-4: 文档 nit 非 bug；L439-442 设计决策注释已解释 writeCtx 用途

## 2026-04-16 slot=1 TARGET=xetcd
- 原始发现：Claude攻=1 守=5 / Codex A=0(超时无表格) B=0(超时无表格)
- 交叉对抗：Codex攻Claude → a=1 b=4 c=0；Claude攻Codex → N/A(Codex无发现)
- 合议：必修=1 存疑=0 舍弃=4
- 修复：commit 32f95dc
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 | 对抗结果 |
  |---|---|---|---|---|---|---|
  | 1 | FG-H | informer.go:239 | applyEvents 解引 ev.Kv 无 nil 守卫，panic 绕过 defer close 致死锁 | 必修 | 3(CA+CB+Codex-a) | Codex 确认(a) |
  | 2 | FG-H | informer.go:168-178 | watchLoop 无限重试无 MaxRetries | 舍弃 | 1(CB) | Codex 判(b)：Informer 设计即重试至 ctx 取消 |
  | 3 | FG-M | watch.go:512-513 | compaction 恢复注释与实现脱节 | 舍弃 | 1(CB) | Codex 判(b)：逻辑正确 |
  | 4 | FG-M | watch.go:528-531 | sendMaxRetriesErrorIfNeeded 条件冗余 | 舍弃 | 1(CB) | Codex 判(b)：条件有区分作用 |
  | 5 | FG-M | client.go:36-47 | registerWatchGoroutine TOCTOU | 舍弃 | 1(CB) | Codex 判(b)：RLock 已覆盖窗口 |
- 舍弃要点：
  - #2: Informer 文档即"重试至 ctx 取消"，未承诺 MaxRetries（设计决策）
  - #3: compactRev 经 state 保存并在 buildRetryWatchOptions 正确使用，注释可改善但非 bug
  - #4: handleWatchRetry 可因 ctx 取消返回 true，与 MaxRetries 超限条件不同（有区分作用）
  - #5: RLock 覆盖 closed 检查和 watchWg.Add()，Close 写锁保证原子性（代码正确）
