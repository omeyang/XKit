// Package xlimit 提供分布式限流功能，保护系统免受流量过载影响。
//
// # 设计理念
//
// xlimit 基于令牌桶算法实现分布式限流，支持多租户、多维度限流，
// 并在 Redis 故障时自动降级到本地限流。集成 xlog 进行日志记录，
// 集成 xmetrics 进行指标和追踪。
//
// # 核心概念
//
//   - Limiter：限流器接口，支持 Allow/AllowN/Query 操作
//   - Key：限流键，包含租户、调用方、API 等维度信息
//   - Rule：限流规则，定义限流窗口、配额和覆盖策略
//   - Result：限流结果，包含是否允许、剩余配额、重试时间等
//   - QuotaInfo：配额信息，用于查询当前配额状态
//
// # 限流维度
//
// 支持多维度组合限流：
//   - 租户（Tenant）：按租户 ID 限流
//   - 调用方（Caller）：按上游服务限流
//   - API（Method + Path）：按接口限流
//   - 资源（Resource）：按自定义资源名限流
//
// # 层级限流
//
// 支持层级限流策略（串行检查，任一层级拒绝则拒绝）：
//   - 全局限流 → 租户限流 → API 限流
//
// # 快速开始
//
//	// 创建分布式限流器
//	limiter, err := xlimit.New(redisClient,
//	    xlimit.WithRules(
//	        xlimit.TenantRule("tenant-limit", 1000, time.Minute),
//	    ),
//	    xlimit.WithLogger(logger),
//	    xlimit.WithObserver(observer),
//	    xlimit.WithMeterProvider(meterProvider), // 指标收集
//	)
//
//	// 执行限流检查
//	key := xlimit.Key{Tenant: "tenant-001"}
//	result, err := limiter.Allow(ctx, key)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !result.Allowed {
//	    log.Printf("限流触发，请在 %v 后重试", result.RetryAfter)
//	}
//
//	// 查询当前配额（不消耗配额）
//	info, err := limiter.Query(ctx, key)
//	if err == nil {
//	    log.Printf("剩余配额: %d/%d", info.Remaining, info.Limit)
//	}
//
// # HTTP 中间件
//
//	mux := http.NewServeMux()
//	mux.Handle("/api/", xlimit.HTTPMiddleware(limiter)(handler))
//
// # 降级策略
//
// Redis 故障时支持三种降级策略：
//   - FallbackLocal：降级到本地限流（推荐）
//   - FallbackOpen：放行所有请求
//   - FallbackClose：拒绝所有请求
//
// 也支持自定义降级函数：
//
//	xlimit.WithCustomFallback(func(ctx context.Context, key xlimit.Key, n int, err error) (*xlimit.Result, error) {
//	    // 自定义降级逻辑
//	    return &xlimit.Result{Allowed: true}, nil
//	})
//
// # 动态 Pod 数量
//
// 本地降级时支持动态获取 Pod 数量：
//
//	// 从环境变量获取
//	xlimit.WithPodCountProvider(xlimit.NewEnvPodCount("POD_COUNT", 4))
//
//	// 静态配置
//	xlimit.WithPodCount(4)
//
// # 配置管理
//
// 支持从 xconf 加载配置并支持热更新：
//
//	provider := xlimit.NewXConfProvider(cfg, "ratelimit")
//	limiter, _ := xlimit.New(redisClient,
//	    xlimit.WithConfigProvider(provider),
//	)
//
// # 可观测性
//
// xlimit 集成 xlog 和 xmetrics 提供完整的可观测性：
//
// 日志（xlog）：
//   - Debug：限流通过事件
//   - Warn：限流拒绝和降级事件
//
// 追踪（xmetrics.Observer）：
//   - 每次限流检查创建 span
//   - 记录 limiter.type、request.count、allowed、rule 等属性
//
// 指标（OpenTelemetry Metrics）：
//   - xlimit.requests.total：请求总数 (Counter)
//   - xlimit.denied.total：被拒绝请求数 (Counter)
//   - xlimit.fallback.total：降级次数 (Counter)
//   - xlimit.check.duration：检查延迟 (Histogram)
package xlimit
