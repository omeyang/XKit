// Package xauth 提供认证服务客户端，用于获取和管理访问 Token、平台信息等。
//
// # 功能概述
//
// xauth 包提供以下核心功能：
//   - Token 管理：获取、刷新、验证访问 Token
//   - 平台信息：获取平台 ID、判断是否有父平台、获取未归类组 Region ID
//   - 带认证的 HTTP 请求：自动添加 Authorization 头
//   - 双层缓存：本地缓存 + Redis 缓存，提升性能
//   - 可观测性：集成日志、指标、追踪
//
// # 快速开始
//
// 基本用法：
//
//	// 创建客户端
//	client, err := xauth.NewClient(&xauth.Config{
//	    Host: "https://auth.example.com",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// 获取 Token
//	token, err := client.GetToken(ctx, "tenant-123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 获取平台 ID
//	platformID, err := client.GetPlatformID(ctx, "tenant-123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # 配置选项
//
// 使用 Option 模式配置客户端：
//
//	// 使用 Redis 缓存
//	redisCache := xauth.NewRedisCacheStore(redisClient)
//	client, err := xauth.NewClient(cfg,
//	    xauth.WithCache(redisCache),
//	    xauth.WithLogger(logger),
//	    xauth.WithObserver(observer),
//	    xauth.WithTokenRefreshThreshold(10*time.Minute),
//	)
//
// # 缓存策略
//
// xauth 使用双层缓存策略：
//   - L1 本地缓存：基于 xlru（LRU + TTL），提供高性能本地访问
//   - L2 Redis 缓存：分布式缓存，支持多实例共享
//
// L1 缓存设计决策：
// 使用 xlru 而非 sync.Map 的原因：
//   - xlru 内置 TTL 过期机制，无需手动检查过期时间
//   - xlru 内置 LRU 淘汰策略，自动管理缓存容量
//   - xlru 完全并发安全，避免 sync.Map 的竞态窗口问题
//
// Token 缓存 TTL 根据 Token 有效期动态计算，在 Token 即将过期前会触发后台刷新。
//
// 短期 Token 处理：
//   - Token 有效期 > 刷新阈值：正常缓存（TTL = 有效期 - 刷新阈值）
//   - Token 有效期 <= 刷新阈值但 > 11秒：使用实际有效期减去 10 秒安全边际
//   - Token 有效期 <= 11秒：极短期 Token 不缓存到 Redis，仅本地使用
//
// # 并发安全
//
// xauth 使用 singleflight 防止缓存击穿：
//   - 同一 tenantID 的并发请求只会触发一次实际 API 调用
//   - Token 获取和平台信息获取都有 singleflight 保护
//
// # 与 xctx 集成
//
// xauth 可以与 xctx 包集成，从 context 获取租户信息：
//
//	// 从 context 获取租户 ID 并获取 Token
//	token, err := client.GetTokenFromContext(ctx)
//
//	// 预加载平台信息到 context
//	ctx, err = xauth.WithPlatformInfo(ctx, client, tenantID)
//
// # 错误处理
//
// xauth 定义了丰富的错误类型，支持错误分类和重试判断：
//
//	err := client.GetToken(ctx, tenantID)
//	if errors.Is(err, xauth.ErrTokenExpired) {
//	    // Token 已过期
//	}
//	if xauth.IsRetryable(err) {
//	    // 可重试的错误（如网络超时、服务端错误）
//	}
//
// # 可观测性
//
// xauth 支持可观测性集成：
//   - 日志：使用 log/slog，自动注入 trace_id/tenant_id
//   - 指标：操作计数、延迟、错误率
//   - 追踪：HTTP 请求自动传播 trace context，包括：
//   - GetToken 操作追踪
//   - VerifyToken 操作追踪
//   - HTTP 请求追踪（method、url）
//
// 配置 Observer：
//
//	client, err := xauth.NewClient(cfg,
//	    xauth.WithObserver(observer),  // xmetrics.Observer
//	    xauth.WithLogger(logger),      // *slog.Logger
//	)
//
// # 迁移指南
//
// 从 gobase/xdrauth 迁移：
//
//	// 旧代码
//	err := xdrauth.WithAuthReq(ctx, tenantID, url, "GET", nil, nil, &resp)
//
//	// 新代码
//	err := client.Request(ctx, &xauth.AuthRequest{
//	    TenantID: tenantID,
//	    URL:      url,
//	    Method:   "GET",
//	    Response: &resp,
//	})
//
// 功能对应关系：
//   - xdrauth.GetPlatformID() -> client.GetPlatformID()
//   - xdrauth.HaveParentPlatform() -> client.HasParentPlatform()
//   - xdrauth.GetUnclassRegionID() -> client.GetUnclassRegionID()
//   - xdrauth.WithAuthReq() -> client.Request()
//
// # 迁移注意事项
//
// URL 处理：
// Request 方法的 URL 参数支持两种格式：
//   - 相对路径（如 "/api/users"）：会与 baseURL 拼接
//   - 完整 URL（如 "https://other-host.com/api/users"）：直接使用
//
// 这意味着旧代码中使用完整 URL 的场景可以直接迁移，无需修改。
//
// Token 验证行为变化：
// 旧版 xdrauth.WithAuthReq 使用双检锁模式：先检查 Token 是否存在，
// 如果存在则调用 verify 验证有效性，验证失败才获取新 Token。
// 这意味着每次请求都有一次 HTTP 调用（verify）的开销。
//
// 新版 Request 不再每次验证，而是依赖 Token 过期时间（ExpiresAt）和后台刷新。
// 这大幅减少了 HTTP 请求数量，但如果服务端主动吊销 Token，业务请求会先收到 401。
//
// 可以启用 EnableAutoRetryOn401 选项来自动处理 Token 吊销场景：
//
//	client, err := xauth.NewClient(cfg,
//	    xauth.WithAutoRetryOn401(true),  // 遇到 401 自动清除缓存并重试
//	)
//
// 启用后，Request 方法遇到 401 错误时会自动清除 Token 缓存并重试一次。
// 这是性能优化，避免了每次请求的额外 HTTP 调用（旧版的 verify 行为）。
//
// 全局包级 API 变化：
// 旧版提供包级函数（xdrauth.WithAuthReq, xdrauth.GetPlatformID 等），
// 通过内部全局单例 gAuth 实现。
// 新版采用实例模式，需要先创建 Client 再调用方法。
// 如果业务需要全局访问，可以自行封装：
//
//	var globalClient xauth.Client
//
//	func init() {
//	    globalClient = xauth.MustNewClient(&xauth.Config{Host: "..."})
//	}
//
//	func GetToken(ctx context.Context, tenantID string) (string, error) {
//	    return globalClient.GetToken(ctx, tenantID)
//	}
//
// API 返回类型变化：
//   - HasParentPlatform 返回 (bool, error)，旧版 HaveParentPlatform 返回 (string, error)
//
// API Key Token TTL 策略变化：
// 旧版 API Key Token 没有 ExpiresIn 信息，完全依赖服务端 verify 判断有效性。
// 新版为 API Key Token 设置默认 6 小时有效期（DefaultTokenCacheTTL），
// 依赖缓存 TTL 和后台刷新机制。如果服务端 Token 实际有效期更长，
// 新版会在 6 小时后重新获取，这可能导致不必要的刷新。
// 如果服务端 Token 有效期更短，则需要启用 WithAutoRetryOn401(true) 处理 401 重试
//
// 缓存设计定位：
// Redis 缓存主要用于进程内缓存持久化，减少服务重启时的冷启动延迟。
// 由于 Token 的 ExpiresAt 字段不会序列化到 Redis，跨进程共享 Token 缓存时需注意：
// 从 Redis 读取的 Token 会根据 ExpiresIn 重新计算 ExpiresAt。
//
// # TLS 安全配置
//
// 重要：默认配置跳过证书验证（InsecureSkipVerify: true），这是为了与 gobase/xdrauth
// 原实现保持向后兼容。生产环境强烈建议配置安全的 TLS 选项。
//
// 生产环境推荐配置：
//
//	client, err := xauth.NewClient(&xauth.Config{
//	    Host: "https://auth.example.com",
//	    TLS: &xauth.TLSConfig{
//	        InsecureSkipVerify: false,           // 启用证书验证
//	        RootCAFile:         "/path/to/ca.pem", // CA 证书
//	    },
//	})
//
// 使用自定义证书（mTLS）：
//
//	client, err := xauth.NewClient(&xauth.Config{
//	    Host: "https://auth.example.com",
//	    TLS: &xauth.TLSConfig{
//	        InsecureSkipVerify: false,
//	        RootCAFile:         "/path/to/ca.pem",
//	        CertFile:           "/path/to/client.pem",
//	        KeyFile:            "/path/to/client-key.pem",
//	    },
//	})
//
// 仅开发/测试环境可使用默认配置或 NewSkipVerifyHTTPClient：
//
//	// 开发环境：使用默认配置（跳过证书验证）
//	client, err := xauth.NewClient(&xauth.Config{
//	    Host: "https://auth.dev.example.com",
//	})
//
// # 默认行为说明
//
// 以下是一些关键的默认行为，请特别注意：
//
// TLS 配置：
//   - Config.TLS 为 nil 时，默认跳过证书验证（InsecureSkipVerify: true）
//   - 这是为了与 gobase/xdrauth 保持向后兼容
//   - 生产环境强烈建议显式配置 TLS 选项
//
// Logger 配置：
//   - WithLogger(nil) 会使用 slog.Default()，而非禁用日志
//   - 如需禁用日志，请使用 slog.New(slog.NewTextHandler(io.Discard, nil))
//
// # 扩展指南
//
// xauth 提供多个扩展点：
//
// MetricsRecorder 接口：
//   - 用于自定义细粒度指标收集
//   - 可实现按租户、Token 类型等维度的统计
//   - 默认使用 xmetrics.Observer 提供基础可观测性
//
// CacheStore 接口：
//   - 用于自定义远程缓存实现
//   - 默认提供 NoopCacheStore（无缓存）和 RedisCacheStore
//   - 可实现自定义缓存后端（如 Memcached）
//
// HTTP 客户端：
//   - 通过 WithHTTPClient 注入自定义 HTTP 客户端
//   - 可用于添加自定义中间件、重试逻辑等
//
// # Graceful Shutdown
//
// client.Close() 会执行以下清理：
//   - 取消所有后台刷新任务
//   - 清理本地缓存
//
// 建议在应用退出时调用 Close()：
//
//	client, _ := xauth.NewClient(cfg)
//	defer client.Close()
package xauth
