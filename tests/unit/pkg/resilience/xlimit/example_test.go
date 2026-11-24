package xlimit_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/resilience/xlimit"
)

func Example_tenantRateLimit() {
	// 创建 Redis 客户端（示例使用 miniredis）
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器，配置租户级限流规则
	limiter, err := xlimit.New(client,
		xlimit.WithRules(
			xlimit.TenantRule("tenant-limit", 100, time.Minute),
		),
		xlimit.WithFallback(""), // 禁用降级，仅使用分布式限流
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	// 执行限流检查
	ctx := context.Background()
	key := xlimit.Key{Tenant: "tenant-001"}

	result, err := limiter.Allow(ctx, key)
	if err != nil {
		fmt.Println("限流检查失败:", err)
		return
	}

	if result.Allowed {
		fmt.Println("请求通过")
	} else {
		fmt.Println("请求被限流")
	}
	// Output: 请求通过
}

func Example_tenantOverride() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器，VIP 租户有更高配额
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.Rule{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       100,
			Window:      time.Minute,
			Overrides: []xlimit.Override{
				{Match: "tenant:vip-corp", Limit: 1000},
				{Match: "tenant:vip-*", Limit: 500},
			},
		}),
		xlimit.WithFallback(""),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	ctx := context.Background()

	// 普通租户
	result1, err := limiter.Allow(ctx, xlimit.Key{Tenant: "normal-user"})
	if err != nil {
		fmt.Println("限流检查失败:", err)
		return
	}
	fmt.Println("普通租户配额:", result1.Limit)

	// VIP 精确匹配
	result2, err := limiter.Allow(ctx, xlimit.Key{Tenant: "vip-corp"})
	if err != nil {
		fmt.Println("限流检查失败:", err)
		return
	}
	fmt.Println("VIP-corp 配额:", result2.Limit)

	// VIP 通配匹配
	result3, err := limiter.Allow(ctx, xlimit.Key{Tenant: "vip-enterprise"})
	if err != nil {
		fmt.Println("限流检查失败:", err)
		return
	}
	fmt.Println("VIP-enterprise 配额:", result3.Limit)

	// Output:
	// 普通租户配额: 100
	// VIP-corp 配额: 1000
	// VIP-enterprise 配额: 500
}

func Example_localLimiter() {
	// 创建本地限流器（不依赖 Redis）
	limiter, err := xlimit.NewLocal(
		xlimit.WithRules(
			xlimit.TenantRule("tenant-limit", 10, time.Second),
		),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	ctx := context.Background()
	key := xlimit.Key{Tenant: "local-tenant"}

	// 快速消耗配额
	for i := range 12 {
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			fmt.Println("限流检查失败:", err)
			return
		}
		if !result.Allowed {
			fmt.Println("第", i+1, "个请求被限流")
			break
		}
	}
	// Output: 第 11 个请求被限流
}

func Example_ruleBuilder() {
	// 使用构建器创建复杂规则
	rule := xlimit.NewRuleBuilder("api-limit").
		KeyTemplate("tenant:${tenant_id}:api:${method}:${path}").
		Limit(100).
		Window(time.Second).
		Burst(150).
		AddOverride("tenant:vip-*:api:*:*", 500).
		AddOverrideWithWindow("tenant:*:api:POST:/v1/orders", 10, time.Second).
		Build()

	fmt.Println("规则名称:", rule.Name)
	fmt.Println("默认配额:", rule.Limit)
	fmt.Println("突发容量:", rule.Burst)
	fmt.Println("覆盖规则数:", len(rule.Overrides))
	// Output:
	// 规则名称: api-limit
	// 默认配额: 100
	// 突发容量: 150
	// 覆盖规则数: 2
}

func Example_keyBuilder() {
	// 使用链式调用构建限流键
	key := xlimit.Key{}.
		WithTenant("tenant-001").
		WithCaller("order-service").
		WithMethod("POST").
		WithPath("/v1/orders").
		WithExtra("region", "us-east-1")

	fmt.Println("键字符串:", key.String())

	// 渲染模板
	template := "tenant:${tenant_id}:caller:${caller_id}:api:${method}:${path}"
	fmt.Println("渲染结果:", key.Render(template))
	// Output:
	// 键字符串: tenant=tenant-001,caller=order-service,method=POST,path=/v1/orders,region=us-east-1
	// 渲染结果: tenant:tenant-001:caller:order-service:api:POST:/v1/orders
}

func Example_callback() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器，配置回调
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 2, time.Minute)),
		xlimit.WithFallback(""),
		xlimit.WithOnAllow(func(key xlimit.Key, result *xlimit.Result) {
			fmt.Printf("允许: tenant=%s, remaining=%d\n", key.Tenant, result.Remaining)
		}),
		xlimit.WithOnDeny(func(key xlimit.Key, result *xlimit.Result) {
			fmt.Printf("拒绝: tenant=%s, rule=%s\n", key.Tenant, result.Rule)
		}),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	ctx := context.Background()
	key := xlimit.Key{Tenant: "callback-tenant"}

	// 发送 3 个请求（回调函数会处理输出）
	for range 3 {
		_, _ = limiter.Allow(ctx, key) //nolint:errcheck // 示例：结果由回调处理
	}
	// Output:
	// 允许: tenant=callback-tenant, remaining=1
	// 允许: tenant=callback-tenant, remaining=0
	// 拒绝: tenant=callback-tenant, rule=tenant-limit
}

func Example_httpMiddleware() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 2, time.Minute)),
		xlimit.WithFallback(""),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	// 创建 HTTP 中间件
	middleware := xlimit.HTTPMiddleware(limiter)

	// 创建处理器
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "OK") //nolint:errcheck // HTTP handler: 写入错误通常表示客户端断开
	}))

	// 发送请求
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "test-tenant")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code == http.StatusOK {
			fmt.Println("请求", i+1, "通过")
		} else {
			fmt.Println("请求", i+1, "被限流")
		}
	}
	// Output:
	// 请求 1 通过
	// 请求 2 通过
	// 请求 3 被限流
}

func Example_grpcInterceptor() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 100, time.Minute)),
		xlimit.WithFallback(""),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	// 创建 gRPC 拦截器
	unaryInterceptor := xlimit.UnaryServerInterceptor(limiter)
	streamInterceptor := xlimit.StreamServerInterceptor(limiter)

	// 输出拦截器类型（验证创建成功）
	fmt.Printf("unary interceptor: %T\n", unaryInterceptor)
	fmt.Printf("stream interceptor: %T\n", streamInterceptor)

	// Output:
	// unary interceptor: grpc.UnaryServerInterceptor
	// stream interceptor: grpc.StreamServerInterceptor
}

func Example_quotaQuery() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 10, time.Minute)),
		xlimit.WithFallback(""),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	ctx := context.Background()
	key := xlimit.Key{Tenant: "query-tenant"}

	// 先消耗一些配额（仅用于设置测试状态，错误不影响示例）
	_, _ = limiter.Allow(ctx, key) //nolint:errcheck // 示例：仅用于消耗配额
	_, _ = limiter.Allow(ctx, key) //nolint:errcheck // 示例：仅用于消耗配额

	// 查询当前配额（不消耗配额）
	querier, ok := limiter.(xlimit.Querier)
	if !ok {
		fmt.Println("查询失败: limiter does not support Query")
		return
	}
	info, err := querier.Query(ctx, key)
	if err != nil {
		fmt.Println("查询失败:", err)
		return
	}

	fmt.Println("规则:", info.Rule)
	fmt.Println("配额上限:", info.Limit)
	fmt.Println("剩余配额:", info.Remaining)

	// Output:
	// 规则: tenant-limit
	// 配额上限: 10
	// 剩余配额: 8
}

func Example_dynamicPodCount() {
	// 创建本地限流器，配置动态 Pod 数量
	limiter, err := xlimit.NewLocal(
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 100, time.Second)),
		// 从环境变量获取 Pod 数量，默认 4
		xlimit.WithPodCountProvider(xlimit.NewEnvPodCount("POD_COUNT", 4)),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	ctx := context.Background()
	key := xlimit.Key{Tenant: "pod-count-tenant"}

	result, err := limiter.Allow(ctx, key)
	if err != nil {
		fmt.Println("限流检查失败:", err)
		return
	}
	// 本地配额 = 100 / 4 = 25
	fmt.Println("本地配额:", result.Limit)

	// Output:
	// 本地配额: 25
}

func Example_customFallback() {
	// 创建 Redis 客户端
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 创建限流器，配置自定义降级
	limiter, err := xlimit.New(client,
		xlimit.WithRules(xlimit.TenantRule("tenant-limit", 100, time.Minute)),
		xlimit.WithFallback(xlimit.FallbackLocal),
		xlimit.WithCustomFallback(func(_ context.Context, key xlimit.Key, _ int, _ error) (*xlimit.Result, error) {
			// 自定义降级逻辑：VIP 租户放行，其他拒绝
			if key.Tenant == "vip" {
				return &xlimit.Result{Allowed: true, Rule: "custom-fallback"}, nil
			}
			return &xlimit.Result{Allowed: false, Rule: "custom-fallback"}, nil
		}),
	)
	if err != nil {
		fmt.Println("创建限流器失败:", err)
		return
	}
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup: Close 错误在清理时无法有效处理

	fmt.Println("自定义降级限流器创建成功")

	// Output:
	// 自定义降级限流器创建成功
}
