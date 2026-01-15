package xlimit

import (
	"net/http"
)

// HTTPKeyExtractor 从 HTTP 请求中提取限流键
type HTTPKeyExtractor struct {
	tenantHeader      string
	callerHeader      string
	pathNormalizer    func(string) string
	resourceExtractor func(*http.Request) string
	extraExtractor    func(*http.Request) map[string]string
}

// HTTPKeyExtractorOption HTTP 键提取器选项
type HTTPKeyExtractorOption func(*HTTPKeyExtractor)

// DefaultHTTPKeyExtractor 创建默认的 HTTP 键提取器
// 默认从 X-Tenant-ID 和 X-Caller-ID header 中提取信息
func DefaultHTTPKeyExtractor() *HTTPKeyExtractor {
	return &HTTPKeyExtractor{
		tenantHeader: "X-Tenant-ID",
		callerHeader: "X-Caller-ID",
	}
}

// NewHTTPKeyExtractor 创建自定义的 HTTP 键提取器
func NewHTTPKeyExtractor(opts ...HTTPKeyExtractorOption) *HTTPKeyExtractor {
	e := DefaultHTTPKeyExtractor()
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Extract 从 HTTP 请求中提取限流键
func (e *HTTPKeyExtractor) Extract(r *http.Request) Key {
	if r == nil {
		return Key{}
	}

	key := Key{
		Tenant: r.Header.Get(e.tenantHeader),
		Caller: r.Header.Get(e.callerHeader),
		Method: r.Method,
		Path:   r.URL.Path,
	}

	// 应用路径规范化器
	if e.pathNormalizer != nil {
		key.Path = e.pathNormalizer(key.Path)
	}

	// 提取资源信息
	if e.resourceExtractor != nil {
		key.Resource = e.resourceExtractor(r)
	}

	// 提取额外信息
	if e.extraExtractor != nil {
		key.Extra = e.extraExtractor(r)
	}

	return key
}

// WithTenantHeader 设置租户 ID 的 header 名称
func WithTenantHeader(header string) HTTPKeyExtractorOption {
	return func(e *HTTPKeyExtractor) {
		e.tenantHeader = header
	}
}

// WithCallerHeader 设置调用方 ID 的 header 名称
func WithCallerHeader(header string) HTTPKeyExtractorOption {
	return func(e *HTTPKeyExtractor) {
		e.callerHeader = header
	}
}

// WithPathNormalizer 设置路径规范化器
// 用于将动态路径转换为模式，例如 /users/123 -> /users/:id
func WithPathNormalizer(normalizer func(string) string) HTTPKeyExtractorOption {
	return func(e *HTTPKeyExtractor) {
		e.pathNormalizer = normalizer
	}
}

// WithResourceExtractor 设置资源提取器
// 用于从请求中提取资源标识
func WithResourceExtractor(extractor func(*http.Request) string) HTTPKeyExtractorOption {
	return func(e *HTTPKeyExtractor) {
		e.resourceExtractor = extractor
	}
}

// WithExtraExtractor 设置额外信息提取器
// 用于从请求中提取自定义信息
func WithExtraExtractor(extractor func(*http.Request) map[string]string) HTTPKeyExtractorOption {
	return func(e *HTTPKeyExtractor) {
		e.extraExtractor = extractor
	}
}
