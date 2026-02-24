// Package xmetrics 提供统一的可观测性接口（metrics + tracing）。
//
// # 设计理念
//
// xmetrics 仅定义最小化接口：[Observer] / [Span] / [Attr]，
// 业务代码只依赖接口；具体实现可替换。
// 默认实现基于 OpenTelemetry，兼容主流可观测栈。
//
// 当 Observer 为 nil 时，[Start] 返回零开销的 [NoopSpan]，
// 可安全用于不需要可观测性的场景。
//
// # 快速上手
//
//	obs, err := xmetrics.NewOTelObserver()
//	// ... handle err ...
//	ctx, span := xmetrics.Start(ctx, obs, xmetrics.SpanOptions{
//	    Component: "myservice",
//	    Operation: "do_work",
//	    Kind:      xmetrics.KindClient,
//	})
//	defer func() { span.End(xmetrics.Result{Err: bizErr}) }()
//	result, bizErr = doWork(ctx)
//
// # 统一指标
//
//   - xkit.operation.total — 操作计数（Counter，unit="{operation}"）
//   - xkit.operation.duration — 操作耗时（Histogram，unit="s"，单位：秒）
//
// duration Histogram 默认桶边界为 [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]（秒），
// 适用于典型 API 操作（1ms ~ 10s）。可通过 [WithHistogramBuckets] 自定义。
// 自定义桶边界必须为非负值、严格递增且不含 NaN/Inf，否则 [NewOTelObserver] 返回 [ErrInvalidBuckets]。
//
// # 统一属性
//
// 每次观测自动附加 metrics 三维度：component / operation / status（见 [AttrKeyComponent] 等常量）。
//
// 设计决策: metrics 仅记录 component/operation/status 三个低基数维度；
// 通过 [SpanOptions.Attrs] 和 [Result.Attrs] 添加的自定义属性仅出现在 trace span 上。
// 这避免了 metrics 因高基数自定义属性导致的时序膨胀。
//
// component / operation / status 是保留属性键，自定义属性中使用这些键会被静默过滤，
// 以防止用户属性覆盖系统属性导致 trace 与 metrics 数据不一致。
//
// # component / operation 使用约束
//
// component 和 operation 应为静态的低基数字符串（如服务名、方法名），
// 禁止将动态值（如请求 ID、用户 ID、URL 路径参数）写入。
// 违反此约束会导致 metrics 时序爆炸和存储成本上升。
//
// operation 推荐使用 snake_case 命名（如 "find_page"、"produce"），
// 与 OTel 语义约定保持一致，便于跨包查询和仪表盘维护。
//
// # Status 语义
//
// [Result.Status] 仅接受 [StatusOK] / [StatusError] 两种值。
// 未知 Status 值不会透传到 metrics，而是回退到 [Result.Err] 推导逻辑。
//
// # Attr 构造
//
// 推荐使用类型安全的构造函数（[String]、[Bool]、[Int] 等）创建属性。
// [Any] 函数会对未知类型调用 fmt.Sprint 字符串化，可能产生不稳定或过大的值，
// 仅在确需存放非标准类型时使用。
//
// # 适用范围
//
// xmetrics 面向简单的"操作级别"观测（一次操作 = 一个 span + counter + histogram）。
// 需要多 Counter、Gauge、独立 Histogram 等复杂观测的包（如 xsemaphore）
// 应直接使用 OTel API 以获得完整控制力。
package xmetrics
