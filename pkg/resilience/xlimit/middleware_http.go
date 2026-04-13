package xlimit

import (
	"net/http"
)

// HTTPMiddleware 创建 HTTP 限流中间件
//
// 示例:
//
//	limiter, _ := xlimit.New(redisClient, xlimit.WithRules(...))
//	mux := http.NewServeMux()
//	mux.Handle("/api/", xlimit.HTTPMiddleware(limiter)(apiHandler))
func HTTPMiddleware(limiter Limiter, opts ...MiddlewareOption) func(http.Handler) http.Handler {
	// 设计决策: nil limiter 使用 panic 而非返回 error。
	// (1) 这是不可恢复的编程错误（构造阶段即可发现），不是运行时异常；
	// (2) 返回 error 会破坏标准 HTTP 中间件签名 func(http.Handler) http.Handler；
	// (3) 与 Go 中间件生态惯例一致（chi、gin 等框架均在 nil 参数时 panic）。
	if limiter == nil {
		panic("xlimit: HTTPMiddleware requires a non-nil Limiter")
	}

	// 应用选项
	mopts := defaultMiddlewareOptions()
	for _, opt := range opts {
		opt(mopts)
	}
	mopts.sanitize()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 检查是否跳过
			if mopts.SkipFunc != nil && mopts.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			// 提取限流键并执行限流检查
			key := mopts.KeyExtractor.Extract(r)
			if handleHTTPLimit(w, r, limiter, mopts, key) {
				return
			}

			// 放行请求
			next.ServeHTTP(w, r)
		})
	}
}

// handleHTTPLimit 执行限流检查并处理结果。
// 返回 true 表示请求已被处理（拒绝），调用方应直接返回。
func handleHTTPLimit(w http.ResponseWriter, r *http.Request, limiter Limiter, mopts *MiddlewareOptions, key Key) bool {
	result, err := limiter.Allow(r.Context(), key)
	if err != nil {
		// 设计决策: 优先检查 result 是否携带拒绝信息（如 FallbackClose 策略
		// 返回 Allowed=false + ErrRedisUnavailable）。仅当 result 为空时
		// 才 fail-open（限流器内部错误不阻塞业务请求）。
		if result != nil && !result.Allowed {
			if mopts.EnableHeaders {
				result.SetHeaders(w)
			}
			mopts.DenyHandler(w, r, result)
			return true
		}
		return false // fail-open
	}

	// 防御性检查: Limiter 接口契约要求 err==nil 时 result 必非 nil，
	// 但第三方实现可能违反契约。此处 fail-open 避免运行时 panic。
	if result == nil {
		return false
	}

	// 添加限流头（如果启用）
	if mopts.EnableHeaders {
		result.SetHeaders(w)
	}

	// 检查是否被限流
	if !result.Allowed {
		mopts.DenyHandler(w, r, result)
		return true
	}

	return false
}

// HTTPMiddlewareFunc 创建 HTTP 限流中间件（函数式）
// 适用于需要 http.HandlerFunc 的场景
func HTTPMiddlewareFunc(limiter Limiter, opts ...MiddlewareOption) func(http.HandlerFunc) http.HandlerFunc {
	middleware := HTTPMiddleware(limiter, opts...)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return middleware(next).ServeHTTP
	}
}
