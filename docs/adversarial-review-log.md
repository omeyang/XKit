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

## 2026-04-17 slot=0 TARGET=xtls
- 原始发现：Claude攻=0 守=3 / Codex A=1 B=0（B 仅搜索未产出表格）
- 交叉对抗：Codex攻Claude → a=3 b=0 c=0；Claude攻Codex → a=1 b=0 c=0
- 合议：必修=3 存疑=0 舍弃=0
- 修复：commit 0b0db32
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CxA-M1/CB-M2 | FG-M | xtls.go:88-101,52 | RequireClientCert=false 提供 CA 时仍强制 mTLS，违反"false 仅 TLS"契约；注释"默认 true"与 Go bool 零值 false 矛盾 | 必修 | 2 |
  | CB-M1 | FG-M | xtls.go:130-134 | BuildClientTLSConfig 强制 CA，不支持回退系统 CA 根（公网 HTTPS 场景） | 必修 | 1 |
  | CB-M3 | FG-M | xtls.go:187-188 | loadCAPool 把空白 PEM 与格式错误统一报 ErrInvalidCA，诊断粒度不足（原标 FG-H 降级：无 panic/leak） | 必修 | 1 |
- 修复要点：
  - 简化 clientAuth 逻辑为 `if c.RequireClientCert { mTLS+loadCA } else { NoClientCert }`；文档改"默认 false"
  - 加 `hasCAMaterial`，CA 可选；未提供时 RootCAs 保持 nil 回退系统根
  - loadCAPool 在 AppendCertsFromPEM 前用 `bytes.TrimSpace` 判空，空白内容返回 ErrMissingCA
  - 补测 TestBuildServer_RequireFalseIgnoresCA / TestBuildServer_EmptyCAFile / TestBuildClient_NoCAUsesSystemRoots

## 2026-04-17 slot=1 TARGET=xauth
- 原始发现：Claude攻=7 (3H/4M) 守=6 (2H/4M) / Codex A=0 B=0（两路 10401/6010 行仅 dump 源码，未产出表格）
- 交叉对抗：Codex攻Claude → a=2 b=10 c=1；Claude攻Codex → N/A（Codex 无发现）
- 合议：必修=1（CA-3/CB-6 合一） 存疑=0 舍弃=12
- 修复：commit <pending>
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CA-3/CB-6 | FG-M | cache.go:158-172 | Redis Hash HSET+EXPIRE 非原子，多字段分批写入时 EXPIRE 反复覆盖 hash TTL，先写字段实际 TTL 被延长 | 必修 | 2 + Codex(a) |
  | CA-1 | FG-H | token.go:119 | wg.Go 疑似非标准 + LoadOrStore 与 goroutine 启动间隙 | 舍弃(FP: Go1.25 已有 WaitGroup.Go；LoadOrStore 已占位) | 1 |
  | CA-2/CB-5 | FG-H/M | token_cache.go:237 | singleflight 闭包捕获首个 ctx | 舍弃(FP: 设计决策已文档化 line 233-236) | 2 |
  | CA-4 | FG-M | token_cache.go:147-189 | Set 修改入参 data race | 舍弃(FP: token 为 loader 新分配，无共享) | 1 |
  | CA-5/CB-2 | FG-M/H | http.go:197 | request 绕过 validateRequestHost SSRF | 舍弃(FP: doc.go:49-56 明确支持跨主机绝对 URL；Do 方法验证是独立入口) | 2 |
  | CA-6/CB-4 | FG-M | platform.go:165,170 | 空字符串作未命中信号 | 舍弃(FP: fetchIDField 空 ID 返回错误，bool 始终 "true"/"false") | 2 |
  | CA-7 | FG-M | token.go:332-395 | 冗余 select + Set 失败 L1 残留旧 token | 舍弃(FP: Set 先写 L1，远端失败不保留旧 L1) | 1 |
  | CB-1/CB-3 | H/M | token.go:117-119 | GetOrLoad ttl 参数与 Set 内重算语义混乱 | 舍弃(FP: Set 注释已说明 ttl 为默认值、内部按 token 重算) | 1 |
- 修复要点：
  - RedisCacheStore 平台数据由 Hash 改为每字段独立 key（`xauth:platform:tenant:field`）
  - SET + EX 单命令原子化；DeletePlatformData/Delete 枚举 `platformAllFields()` 批量删除
  - 新增 TTL 独立性测试 `SetPlatformData fields have independent TTL`，先写字段按自身 TTL 过期不受后写延长

## 2026-04-17 slot=2 TARGET=xconf
- 原始发现：Claude攻=2 (1H/1M) 守=4 (2H/2M) / Codex A=5 (5M) B=4 (4M)
- 交叉对抗：Codex攻Claude → a=0 b=6 c=0（Claude 六条全 FP）；Claude攻Codex → a=8 b=0 c=1
- 合议：必修=6 存疑=0 舍弃=7（Claude 六条全 FP + Codex B4 mapstructure 默认行为不属 M）
- 修复：commit <pending>
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | A1 | FG-M | watch.go:263 | stopped 提前 true，并发 Stop 第二个立即返回但首个仍 Wait，契约违反 | 必修 | 1 + Claude(a) |
  | A2 | FG-M | watch.go:237 | StartAsync runWg.Add(1) 在锁外（runWg.Go 语义），Stop 可在 Add 前 Wait 完并 Close | 必修 | 1 + Claude(a) |
  | A3/B1 | FG-M | watch.go:296 | 回调内 Stop 跳过整个 callbackWg.Wait，多并发 in-flight 回调未等待 | 必修 | 2 + Claude(a) |
  | A4/B2 | FG-M | watch.go:324 | Events/Errors 通道关闭分支不检查 stopped，Stop 后仍触发 unexpected 回调 | 必修 | 2 + Claude(a) |
  | A5 | FG-M | koanf.go:192 | MustUnmarshal 只判 nil 接口，typed-nil Config 在 c.k.Load() 触发不清晰 nil 解引用 | 必修 | 1 + Claude(a) |
  | B3 | FG-M | watch.go:342 | basename 过滤丢弃 K8s ConfigMap `..data` symlink 事件，热更新失效 | 必修 | 1 + Claude(a) |
  | B4 | FG-M | koanf.go:139 | Unmarshal 不清零 target（mapstructure 默认行为） | 舍弃(L 级惯例，非 xconf 契约违反) | 1 |
- 修复要点：
  - Stop 改 stopped + stopDone channel：首个调用方执行清理后 close(stopDone)，非首个等待后返回相同 stopErr；自身是 in-flight 回调时直接返回避免自锁
  - callbackWg + callbackGIDs 合并为 callbackStates map[int64]chan struct{}：每个 in-flight 回调独立 done channel，Stop 只等待非自身
  - StartAsync 将 runWg.Add(1) 移入 mu 锁内，用显式 `go func() { defer Done(); run() }()`
  - run() 通道关闭分支新增 isShuttingDown() 判断，Stop 后静默返回
  - handleEvent 新增 `k8sConfigMapSymlink = "..data"` 允许 K8s atomic symlink swap 触发 Reload
  - MustUnmarshal 用 reflect.ValueOf(cfg).Kind()==Ptr && IsNil() 拒绝 typed-nil，给出清晰 panic 信息
  - 补测 5 条回归：ConcurrentStopIdempotent / StartAsyncStopNoSpuriousCallback / StopWaitsForOtherCallbacks / StopSuppressesChannelClosed / K8sConfigMapSymlink + TypedNilConfig

## 2026-04-17 slot=3 TARGET=xenv
- 原始发现：Claude攻=0 守=5 (1H/4M) / Codex A=1 (1M) B=0(无发现)
- 交叉对抗：Codex攻Claude → a=0 b=4 c=1；Claude攻Codex → a=0 b=0 c=1
- 合议：必修=0 存疑=0 舍弃=6
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CB-1 | FG-M | export_test.go:19-23 | Reset 先清 initialized 后清 globalType 被质疑"锁时序反" | 舍弃(FP: Codex b) | 1 |
  | CB-2 | FG-M | deploy.go:194,234 | Type/RequireType comma-ok 类型断言冗余 | 舍弃(FP: Codex b，lint 要求) | 1 |
  | CB-3 | FG-M | deploy_test.go:631 | TestConcurrentResetAndRead 未显式验证中间态 | 舍弃(FP: Codex c，测试充分性非代码缺陷) | 1 |
  | CB-4 | FG-H | deploy.go:113,165 | Init/InitWith 返回 ErrAlreadyInitialized 隐藏根源 | 舍弃(FP: Codex b，单次初始化契约) | 1 |
  | CB-5 | FG-M | deploy.go:251-257 | xenv.Parse 吞掉 ErrMissingValue 与 xctx 不对齐 | 舍弃(FP: Codex b，deploy.go:251-257 文档化设计决策) | 1 |
  | CxA-1 | FG-M | deploy.go:260 | strings.ToUpper 归一化 Unicode 同形字符(如 U+017F) | 舍弃(FP: Claude c，DEPLOYMENT_TYPE 来自受信 ConfigMap；归一后存储 ASCII 常量 deploy.SaaS，无下游语义变更) | 1 |
- 舍弃要点：
  - CB-1 反序（先清 globalType 后清 initialized）会产生 `initialized=true + globalType=""` 的真中间态，当前顺序配合 `Type() if !initialized.Load() return ""` 和 `RequireType() dt=="" return ErrNotInitialized` 已完整防御 TOCTOU，设计注释已文档化
  - CB-2 `.golangci.yml check-type-assertions: true` 要求 comma-ok 形式，注释已明示；MEMORY 同类模式（xid 等）亦保留
  - CB-4 错误优先级 `ErrAlreadyInitialized > ErrMissing/Empty/Invalid` 为显式设计契约（deploy.go:97,154；xplatform.Init 一致），Init() 仅 main 启动调用一次
  - CB-5 Parse 面向纯解析，空值与非法值统一为 ErrInvalidDeploymentType 是明文设计决策（deploy.go:251-257）；语义区分由上层 Init 通过 ErrMissingEnv/ErrEmptyEnv 完成
  - CxA-1 信任边界分析：DEPLOYMENT_TYPE 为 Kubernetes ConfigMap 环境变量，来源受管理员控制；即使 U+017F 被 ToUpper 归一为 "S"，下游存储仍为 ASCII deploy.SaaS 常量，语义不变；非跨信任边界漏洞，属防御深度建议

## 2026-04-17 slot=4 TARGET=xplatform
- 原始发现：Claude攻=0 守=3 (2H/1M) / Codex A=0(无结论) B=2 (2M)
- 交叉对抗：Codex攻Claude → a=0 b=3 c=0；Claude攻Codex → a=2 b=0 c=0
- 合议：必修=2 存疑=0 舍弃=3
- 修复：commit 42887ef
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CB-1 | FG-M | platform.go:154 | 0x21..0x7e 排除空格 0x20 被指与 gRPC %x20-%x7e 冲突 | 舍弃(FP: Codex b，本包明示禁止空白，比 gRPC 更严是设计决策) | 1 |
  | CB-2 | FG-H | platform.go:102 | TrimSpace+ContainsFunc(IsSpace) 两次判定被指逻辑混淆 | 舍弃(FP: Codex b，先 Missing 后 Invalid 路径清晰) | 1 |
  | CB-3 | FG-H | platform.go:205-208 | Init 二次 TrimSpace 归一化被指与 Validate 分离易遗漏 | 舍弃(FP: Codex b，Validate 只校验不变更，Init 存储前归一化副本) | 1 |
  | CX-B-1 | FG-M | doc.go:13 | 文档未声明仅允许 ASCII 可打印字节，中文 PlatformID 会 Init 失败 | 必修(Claude a) | 1 |
  | CX-B-2 | FG-M | doc.go:14 | 文档未声明 UnclassRegionID 仅允许 ASCII 可打印字节 | 必修(Claude a) | 1 |
- 修复要点：
  - doc.go:13-14 PlatformID/UnclassRegionID 校验规则补充"仅允许 ASCII 可打印字节（0x21..0x7e，不含空格/非 ASCII）"并说明 gRPC imetadata.ValidatePair 约束
  - platform.go Config.PlatformID/UnclassRegionID 字段注释同步补充该条校验规则
  - 仅文档契约精化，实现零改动
- 舍弃要点：
  - CB-1 包契约明确禁止空白字符（含 0x20），ContainsFunc(IsSpace) 预过滤已使 containsNonPrintableASCII 的 0x20 分支不可达，比 gRPC 更严属设计决策
  - CB-2 TrimSpace==""→Missing、ContainsFunc→Invalid 两个分支错误类型不同、语义清晰；测试覆盖两种场景通过验证
  - CB-3 Validate 是值接收者 + 只读；Init 存储前的 TrimSpace 归一化 L205-207 有明确注释，不存在"Validate 通过后直接使用未归一化 cfg"的真实路径（cfg 是值传入，Init 拿到的是调用栈副本，业务侧调用 Validate 后如需继续用需自行决定归一化）

## 2026-04-17 slot=5 TARGET=xtenant
- 原始发现：Claude攻=3 守=3 / Codex A=4 B=1 (去重后 X1-X4 共 4 条)
- 交叉对抗：Codex攻Claude → a=0 b=6 c=0；Claude攻Codex → a=2 b=2 c=0
- 合议：必修=3 存疑=0 舍弃=7
- 修复：commit e64c70e
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | X2 | FG-M | http.go:262,grpc.go:277 | injectPlatform* 先 IsInitialized 再多次 atomic.Load，Reset 并发下读到撕裂快照 | 必修(Codex A + Claude CC a) | 2 |
  | X4 | FG-M | doc.go:154 | 文档"Extract* 线程安全"未限定调用方对 Header/MD 的并发写约束 | 必修(Codex A + Claude CC a) | 2 |
  | X1 | FG-M | grpc.go:302 | 非 ASCII 租户值写入 outgoing metadata 由 grpc-go 发送前拒绝(Internal)，doc 未警示 | 必修(文档)(Codex A+B，Claude CC b→采纳为文档契约) | 2 |
  | X3 | FG-M | grpc.go:448 | status.Error 返回值丢失 sentinel 错误链 errors.Is 不匹配 | 舍弃(FP: Claude CC b，gRPC 惯例用 status.Code 而非 errors.Is) | 1 |
  | C1 | FG-M | grpc.go:251 | InjectToOutgoingContext hadExisting=false 时返回原 ctx 被指线程安全歧义 | 舍弃(FP: Codex b，无 metadata 可复制且未修改入参，返回原 ctx 不破坏线程安全) | 1 |
  | C2/C6 | FG-M | context.go:147-156 | WithTenantInfo 防御性"不可达"错误处理无测试 | 舍弃(FP: Codex b，xctx 当前仅 nil ctx 报错，前置已拦截；缺测试非缺陷) | 2 |
  | C3 | FG-M | http.go:248 | InjectToRequest 原地写 req.Header 被指文档未说明并发约束 | 舍弃(FP: Codex b，doc.go:158-164 已明示原地写入 API 需独占) | 1 |
  | C4 | FG-H | grpc.go:451 | validateGRPCTenantInfo 未 TrimSpace 被指与 Validate 不一致 | 舍弃(FP: Codex b，私有函数调用前 ExtractFromMetadata 已 TrimSpace) | 1 |
  | C5 | FG-H | http.go:219 | validateHTTPTenantInfo 同上 | 舍弃(FP: Codex b，HTTP 路径同样先 ExtractFromHTTPHeader 已 TrimSpace) | 1 |
- 修复要点：
  - injectPlatformMetadata/injectPlatformHeaders 改用 xplatform.GetConfig 单次快照读，三字段（PlatformID/HasParent/UnclassRegionID）保证来源同一配置实例
  - doc.go 线程安全章节拆分为"Context 操作/Extract 只读/原地写入"三类，明确 Extract* 对入参 Header/MD 的并发写约束需调用方保证
  - doc.go 新增"值约束"章节：gRPC metadata 仅允许 ASCII 可打印字符（%x20-%x7E），非 ASCII 请改用 -bin 后缀 Binary Metadata
  - 新增 TestInjectToRequest_PlatformSnapshotConsistency / TestInjectToOutgoingContext_PlatformSnapshotConsistency 回归测试：并发 Reset+Init 下 200 次迭代校验平台三字段要么全来自同一快照、要么全空
- 舍弃要点：
  - C1-C6 Claude 6 条发现交叉对抗全被 Codex 判 FP：防御性代码在 xctx 契约下真的不可达；TrimSpace 在私有校验函数前的 Extract 链已完成；doc.go 已文档化原地写入约束
  - X3 status.Error(code, msg) 丢失 errors.Is 链在 gRPC 生态并非契约偏离：业界惯例用 status.Code(err)/status.FromError(err) 解析而非 errors.Is；修复需引入自定义 GRPCStatus+Unwrap 复合类型，收益/复杂度不匹配

## 2026-04-18 slot=0 TARGET=xdbg
- 原始发现：Claude攻=5 守=5 / Codex A=0(纯搜索日志无表格) B=0(纯搜索日志无表格)
- 交叉对抗：Codex攻Claude → 超时无输出；Claude攻Codex → N/A(Codex 0 发现)
- 合议：必修=0 存疑=0 舍弃=10
- 修复："无发现"
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CA-1 | FG-M | command_builtin.go:305 | os.CreateTemp("") 依赖隐式行为 | 舍弃(FP: os.CreateTemp("") 标准 Go 行为，内部调用 os.TempDir()) | 1 |
  | CA-2 | FG-M | session.go:287-303 | conn.Close() 错误未检查 | 舍弃(FP: Close() 在 line 302 返回 error) | 1 |
  | CA-3 | FG-M | server.go:18 | Stopped 终态 goroutine 退出 | 舍弃(FP: Stop() 调用 waitForGoroutines() 确保退出) | 1 |
  | CA-4 | FG-H | options.go:339-348 | SocketPerm >0o777 未校验 | 舍弃(FP: setuid/setgid/sticky 是合法 Unix 权限位，函数目的是安全限制非穷举校验) | 1 |
  | CA-5 | FG-M | server.go:124-127 | Enable/Disable CAS 竞态 | 舍弃(FP: CAS 原子操作保证一方成功另一方失败，无混乱) | 1 |
  | CB-1 | FG-M | session.go:181 | 超时判断时序窗口 | 舍弃(FP: 标准 Go ctx.Err() 超时检查模式) | 1 |
  | CB-2 | FG-M | protocol_codec.go:174 | UTF-8 截断越界 | 舍弃(FP: line 169 guard `len(s)<=maxBytes` 防护) | 1 |
  | CB-3 | FG-H | options.go:369 | JSON overhead 突破限制 | 舍弃(FP: sendEncodingErrorResponse 兜底处理) | 1 |
  | CB-4 | FG-M | session.go:255 | 写失败未 cancel | 舍弃(FP: shouldExit() 检查 closed 标志，同步循环下次迭代即退出) | 1 |
  | CB-5 | FG-H | command_builtin.go:90 | exit 异步 Disable 竞态 | 舍弃(FP: wg.Go 追踪 goroutine+audit 日志兜底，设计决策已文档化) | 1 |
- 舍弃要点：
  - CA-1~CA-5 五条攻方发现均经源码核实为 FP：os.CreateTemp/Close 行为符合标准、CAS 原子保证正确、>0o777 是合法权限位、waitForGoroutines 保证清理
  - CB-1~CB-5 五条守方"真问题"均经源码核实为 FP：ctx.Err() 是标准 Go 模式、TruncateUTF8 有 guard、JSON 溢出有 sendEncodingErrorResponse、shouldExit 检查 closed、exit 命令竞态有设计决策文档+审计日志
  - Codex 双路原始扫描均只输出搜索日志（~11K/5.7K 行），未生成发现表格
  - Codex 攻击 Claude 发现的交叉对抗超时（运行 28 分钟无输出），但主编排器独立源码核实已覆盖全部 10 条

## 2026-04-18 slot=1 TARGET=xcron
- 原始发现：Claude攻=5 守=5 / Codex A=0 B=0（双路均只输出源码转储 ~9.5K/3.3K 行，未生成发现表格）
- 交叉对抗：Codex攻Claude → 未执行（Codex 无发现可供攻击）；Claude攻Codex → 未执行（Codex 无发现）；主编排器独立源码核实覆盖全部 10 条
- 合议：必修=0 存疑=0 舍弃=10
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |------|--------|---------|------|------|--------|
  | CA-1 | FG-H | wrapper.go:51-52 | taskCancel 多次调用 | 舍弃(FP: CancelFunc 多次调用为 no-op，Go 标准保证) | 1 |
  | CA-2 | FG-H | locker_redis.go:206,255 | redis.Nil == 比较 | 舍弃(FP: go-redis v9 .Result() 直接返回 sentinel，== 正确；xdlock 用 errors.Is 是不同错误类型) | 1 |
  | CA-3 | FG-M | wrapper.go:72 | taskCtx 二次赋值 cancel 失效 | 舍弃(FP: WithTimeout 创建子 context，父级 cancel 传播到子级) | 1 |
  | CA-4 | FG-M | locker_k8s.go:298 | clockSkew 2s 默认值 | 舍弃(FP: K8s 官方 leader-election 默认值，文档化+允许自定义) | 2 |
  | CA-5 | FG-M | cron.go:156-160 | WithImmediate 浅拷贝 race | 舍弃(FP: opts 创建后只读，浅拷贝仅修改 baseCtx) | 1 |
  | CB-1 | FG-M | wrapper.go:186 | Unlock 用 Background ctx | 舍弃(FP: 设计正确——任务 ctx 取消后仍需释放锁) | 1 |
  | CB-2 | FG-M | locker_redis.go:199-222 | unlockCompat GET-DEL 竞态 | 舍弃(FP: 微秒级窗口，已文档化，TTL 兜底) | 1 |
  | CB-3 | FG-H | cron.go:221-226 | Stop() immediateWg 竞态 | 舍弃(FP: AddJob after Stop 属使用方契约违规) | 1 |
  | CB-4 | FG-H | wrapper.go:311 | renewTimeout min(0,x)=0 | 舍弃(FP: lockTimeout 默认 5s，WithLockTimeout 拒绝 ≤0，fallback 兜底) | 1 |
  | CB-5 | FG-M | locker_k8s.go:291-299 | clockSkew 方向错误 | 舍弃(FP: 方向正确——给持有者更多续期时间防误抢占) | 2 |
- 舍弃要点：
  - CA-1 CancelFunc 幂等调用是 Go context 包标准保证（"After the first call, subsequent calls do nothing"）
  - CA-2 go-redis v9 中 redis.Nil 为 proto.RedisError(string) 类型，.Result() 直接返回不包装，== 比较等效于 errors.Is
  - CA-3 context.WithTimeout(parent, d) 创建 parent 的子 context，cancel parent 传播到所有子级，taskCancel 可控制超时 context
  - CA-4/CB-5 clockSkew 两条合并：默认 2s 对齐 K8s 官方，方向（+clockSkew 延后过期）给持有者更多续期余裕，均为正确设计
  - CB-4 lockTimeout 通过 defaultJobOptions 默认 5s，WithLockTimeout 只接受 >0 值，API 层面无法设为 0
  - Codex 双路原始扫描均只输出源码读取日志（~9.5K/3.3K 行），未生成发现表格

## 2026-04-18 slot=2 TARGET=xdlock
- 原始发现：Claude攻=1 守=4 / Codex A=0(过程日志截断) B=0(过程日志截断)
- 交叉对抗：Codex攻Claude → a=1 b=3 c=1；Claude攻Codex → 无发现可攻
- 合议：必修=0 存疑=0 舍弃=5
- 修复：无发现
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  |---|---|---|---|---|---|
  | CA-1 | FG-M | redis_compat.go:31-32 | nil ctx→Background 替换 | 舍弃(FP: 仅 nil 替换，已取消非 nil ctx 保留) | 1 |
  | CB-1 | FG-H | redis_compat.go:130,133 | fmt.Sprint 非字符串值比对 | 舍弃(FP: redsync 锁值固定为 string) | 1 |
  | CB-2 | FG-M | redis_compat.go:84-89 | evalCompat 脚本识别脆弱 | 舍弃(FP: 未知脚本有防御性回退到 evalLua) | 1 |
  | CB-3 | FG-M | redis_compat.go:156-162 | ctx 取消致 GET-DEL 返回 -1 | 舍弃(FP: DEL/PEXPIRE 取消返回 error 非 -1 + doc.go 已文档化竞态) | 1 |
  | CB-4 | FG-M | redis.go:139-146 | ctx.Err 掩盖 Redis 错误 | 舍弃(FP: L139-142 设计决策注释——redsync 包装 ctx 错误导致匹配失败，优先返回调用方控制信号) | 1 |
- 舍弃要点：
  - CA-1 compatPool.Get() 的 nil→Background 仅处理 nil ctx 边界情况，非 nil 的已取消 ctx 正常传递
  - CB-1 redsync v4 的 GenValueFunc 固定生成 base64 随机字符串，Eval 的 args[0] 永远是 string
  - CB-2 evalCompat 对未知参数数量回退到 evalLua（L119），redsync v4.16 仅 3 个脚本均已覆盖
  - CB-3 doc.go:109-110 已文档化 GET-DEL 微秒级竞态；Redis 命令取消返回 error 非静默返回 -1
  - CB-4 redis.go:139-142 设计决策注释完整说明：redsync 将 context 错误包装在 ErrFailed 中导致 errors.Is 失败，ctx.Err() 独立检查是必要的补偿措施
  - Codex 双路原始扫描均只输出源码读取日志（~8K/5K 行），未生成发现表格

## 2026-04-18 slot=3 TARGET=xhealth
- 原始发现：Claude攻=3 守=5 / Codex A=0 B=0（双路+交叉对抗均只输出源码读取日志，未生成发现表格）
- 交叉对抗：Codex攻Claude → 无输出；Claude攻Codex → 无发现可攻（Codex 0 条）
- 合议：必修=1 存疑=0 舍弃=7
- 修复：commit d34dc31
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 | 对抗结果 |
  |------|--------|---------|------|------|--------|----------|
  | CB-4 | FG-M | check.go:52 | Timeout 负值校验用 ErrInvalidInterval，语义不符 | 必修 | CB+CC 判 a | 已修复 |
  | CA-1 | FG-H | checker.go:86-90 | Body.Close 错误 return 不日志 | 舍弃 | CA 单源，CC 判 b | FP：HTTP 响应 Close 无法补救 |
  | CA-2 | FG-M | health.go:319-322 | singleflight 断言失败 fallback | 舍弃 | CA 单源，CC 判 a 但不可达 | 防御性代码，闭包始终返回 CheckResult |
  | CA-3 | FG-M | health.go:352 | ticker 在 asyncCtx cancel 后继续 | 舍弃 | CA 单源，CC 判 b | stopCh 先于 asyncCancel 关闭 |
  | CB-1 | FG-M | checker.go:40 | conn.Close() 错误忽略 | 舍弃 | CB 单源，CC 判 b | 事实错误：return conn.Close() 已返回 |
  | CB-2 | FG-H | health.go:308 | 闭包 nil error + error check 矛盾 | 舍弃 | CB 单源，CC 判 a 但文档化 | 注释明确前瞻性防御代码 |
  | CB-3 | FG-M | handler.go:22-27 | 路由重复注册 | 舍弃 | CB 单源，CC 判 b | 不同路径不同 handler |
  | CB-5 | FG-M | health.go:296-297 | 异步检查冷启动返回 StatusUp | 舍弃 | CB 单源，CC 判 a 但文档化 | K8s 标准模式，已文档化设计决策 |
- 舍弃要点：
  - CA-1 HTTP 响应体 Body.Close 错误在业界普遍不处理，健康检查结果不受影响
  - CA-2 singleflight 闭包始终返回 (CheckResult, nil)，type assertion fallback 为纯防御性代码
  - CA-3 doShutdown() 先 close(stopCh) 后 asyncCancel()，goroutine 通过 stopCh 退出
  - CB-1 factual error：checker.go:40 为 `return conn.Close()` 已正确返回错误
  - CB-2 health.go:307 注释"闭包始终返回 nil error；若未来修改返回错误则记录以便定位"——前瞻性防御
  - CB-5 health.go:296 注释"异步检查尚未执行，返回默认 up"——K8s startup 标准行为
  - Codex 三次运行（原始 A/B + 交叉对抗）均只输出源码读取日志，未生成结论表格

## 2026-04-18 slot=4 TARGET=xrun
- 原始发现：Claude攻=0 守=0 / Codex A=0（日志截断无结论） B=0（日志截断无结论）
- 交叉对抗：无输入（4 路均无发现），跳过
- 合议：必修=0 存疑=0 舍弃=0
- 修复：无发现
- 备注：Claude 双代理独立扫描 nil/typed-nil/零值契约、并发安全、错误处理、context 传播、资源清理、API 契约、跨平台 build tag 等维度均未发现 FG-H/FG-M 真问题。Codex A 探索了 Plan9 构建失败（syscall.SIGQUIT undefined），判定为非真问题（包面向 Unix/K8s 生命周期管理，Plan9 非目标平台）。Codex A/B 均未输出规范表格，仅产出搜索日志。
