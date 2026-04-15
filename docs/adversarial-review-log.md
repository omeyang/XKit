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

## 2026-04-16 slot=2 TARGET=xmongo
- 原始发现：Claude攻=2 守=2 / Codex A=0(过程日志无结论) B=0(过程日志无结论)
- 交叉对抗：Codex攻Claude → 超时未完成；Claude攻Codex → 无发现可攻
- 合议：必修=1 存疑=0 舍弃=3
- 修复：commit 0fd2083
- 合议表格：

  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CB-1 | FG-M | wrapper.go:497-501 | 无序批量插入失败且 ctx 取消时，原始 MongoDB 错误被 context 错误替换 | 必修(裁决为修) | 1 |
  | CA-1 | FG-M | wrapper.go:426 | BulkResult.Errors 与 resultErr 双返回 | 舍弃(FP:部分成功API惯例) | 1 |
  | CA-2 | FG-H | wrapper.go:269 | SlowQueryInfo.Filter 记录 nil | 舍弃(FP:filter已在L272归一化) | 1 |
  | CB-2 | FG-M | wrapper.go:478 | result nil 检查缺失 | 舍弃(FP:driver契约保证) | 1 |

- 舍弃要点：
  - CA-1: BulkResult.Errors 与 error 返回值是部分成功 API 的惯用模式，非重复
  - CA-2: filter 在 L272-273 已归一化为 bson.D{}，buildSlowQueryInfoFromOps 收到非 nil
  - CB-2: MongoDB driver 契约保证 err==nil 时 result 非 nil

## 2026-04-16 slot=3 TARGET=xetcdtest
- 原始发现：Claude攻=4 守=3 / Codex A=截断无结论 B=截断无结论
- 交叉对抗：Codex无发现可供Claude反攻；Claude发现经源码验证全为FP
- 合议：必修=0 存疑=0 舍弃=7
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CA-1 | FG-H | mock.go:191 | 声称 embed.Etcd.Close() 返回 error 未检查 | 舍弃(FP) | CA+CB |
  | CA-2 | FG-M | mock.go:205 | NewClient 闭包泄漏风险 | 舍弃(FP: 使用模式建议) | CA |
  | CA-3 | FG-M | mock_test.go:25 | context.Background() 无超时 | 舍弃(FP: 测试风格) | CA |
  | CA-4 | FG-M | mock.go:129-134 | panic 恢复 + defer unlock 死锁 | 舍弃(FP: LIFO 顺序正确) | CA |
  | CB-1 | FG-H | mock.go:149-151 | waitReady 内 Close 后 tryStart 重复清理 | 舍弃(FP: removeDir≠Close, 无重复) | CB |
  | CB-2 | FG-M | mock.go:191 | etcd.Close() 无错误处理 | 舍弃(FP: 同 CA-1) | CB |
  | CB-3 | FG-M | mock.go:149 | waitReady 双重 Close 与 tryStart 不一致 | 舍弃(FP: 同 CB-1) | CB |
- 舍弃要点：
  - CA-1/CB-2: embed.Etcd.Close() 签名 func(e *Etcd) Close()，返回 void 非 error
  - CB-1/CB-3: waitReady 调 e.Close() 只关 server/listener，不删 dir；tryStart L150 removeDir 必要且无重复
  - CA-4: defer LIFO 保证 recover 先执行再 unlock，无死锁路径
  - CA-2: 调用方契约 defer cleanup()，非包内缺陷
  - CA-3: 简单 Put/Get 测试无需超时，且其他测试已有 ctx(t, d) 模式

## 2026-04-16 slot=4 TARGET=xredismock
- 原始发现：Claude攻=4 守=3 / Codex A=2 B=截断无结论
- 交叉对抗：Codex攻Claude → 未完成（超时）；Claude攻Codex → a=1 b=1 c=0
- 合议：必修=1 存疑=0 舍弃=6
- 修复：commit e3c9db4
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CxA-M2 | FG-M | mock_test.go:127 | 注释"Redis 不允许空 key"错误，跳过合法空 key 测试 | 必修 | 1 |
  | CA-H1 | FG-H | mock.go:40,43 | Client()/Server()Close后无检查 | 舍弃(FP:测试辅助包契约) | 2 |
  | CA-H2 | FG-H | mock.go:48-62 | Close原子性 | 舍弃(FP:整个Close在mutex内) | 1 |
  | CA-M1 | FG-M | mock.go:72 | cleanup引用泄漏 | 舍弃(FP:GC回收+已有测试) | 2 |
  | CA-M2 | FG-M | mock.go:55-62 | Close错误未join | 舍弃(FP:文档化设计决策L46) | 2 |
  | CB-H1 | FG-H | mock.go:56 | 错误被吞 | 舍弃(FP:同CA-M2) | 1 |
  | CxA-M1 | FG-M | mock_test.go:119 | Fuzz状态累积 | 舍弃(FP:Set/Get无状态依赖) | 1 |
- 舍弃要点：
  - CA-H1/CB-M1: 测试辅助包，调用方契约 defer m.Close()，Close 后使用非包内缺陷
  - CA-H2: Close 全程在 mutex 内，closed=true 在资源关闭前设置，无间隙
  - CA-M2/CB-H1: Close() 返回 void 已文档化（L46），对齐 testing.TB.Cleanup 惯例
  - CxA-M1: Fuzz 每轮 Set/Get 独立语义，共享实例不影响正确性

## 2026-04-16 slot=5 TARGET=xfile
- 原始发现：Claude攻=2 守=1 / Codex A=0 B=0（两路均未产出结论表格，仅 dump 源码后 go 命令不可用）
- 交叉对抗：Codex攻Claude → Codex 卡 stdin 未产出；Claude攻Codex → Codex 无发现，N/A
- 合议：必修=0 存疑=0 舍弃=2
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CA-H1 | FG-H | path.go:303,308 | 双%w包装错误链 | 舍弃(FP:Go1.20+标准多%w特性，项目约定) | 1 |
  | CA-M1/CB-M1 | FG-M | path.go:393 | 循环边界不对称 | 舍弃(FP:两函数均迭代256次，语法不同语义相同) | 2 |
- 舍弃要点：
  - CA-H1: fmt.Errorf 双 %w 是 Go 1.20+ 标准特性，errors.Is 可匹配两个 cause；xid/xmac 已有先例
  - CA-M1/CB-M1: evalSymlinksPartial `i>255` 与 rejectDanglingSymlink `i<=255` 均允许 i=0..255 共 256 次迭代，边界对称
