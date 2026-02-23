package xauth

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

const (
	// maxResponseSize 最大响应体大小（10MB）。
	// 防止恶意或异常响应导致内存溢出。
	maxResponseSize = 10 * 1024 * 1024
)

// =============================================================================
// HTTP 客户端
// =============================================================================

// HTTPClient 封装的 HTTP 客户端。
type HTTPClient struct {
	client   *http.Client
	baseURL  string
	timeout  time.Duration
	observer xmetrics.Observer
}

// HTTPClientConfig HTTP 客户端配置。
type HTTPClientConfig struct {
	// BaseURL 基础 URL。
	BaseURL string

	// Timeout 请求超时时间。
	Timeout time.Duration

	// TLSConfig TLS 配置。
	TLSConfig *tls.Config

	// Client 自定义 HTTP 客户端。
	// 如果设置，其他配置将被忽略。
	Client *http.Client

	// Observer 可观测性接口。
	// 用于记录 HTTP 请求的指标和追踪信息。
	Observer xmetrics.Observer
}

// NewHTTPClient 创建 HTTP 客户端。
func NewHTTPClient(cfg HTTPClientConfig) *HTTPClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}

	var client *http.Client
	if cfg.Client != nil {
		client = cfg.Client
	} else {
		transport := &http.Transport{
			TLSClientConfig:     cfg.TLSConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		}
	}

	observer := cfg.Observer
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}

	return &HTTPClient{
		client:   client,
		baseURL:  cfg.BaseURL,
		timeout:  cfg.Timeout,
		observer: observer,
	}
}

// NewSkipVerifyHTTPClient 创建跳过证书验证的 HTTP 客户端。
// 仅用于开发/测试环境，生产环境请勿使用。
func NewSkipVerifyHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return NewHTTPClient(HTTPClientConfig{
		BaseURL: baseURL,
		Timeout: timeout,
		//nolint:gosec // G402: 本函数专为开发测试设计，函数名已明确说明用途
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		},
	})
}

// Do 执行 HTTP 请求。
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	return c.client.Do(req)
}

// Get 发送 GET 请求。
func (c *HTTPClient) Get(ctx context.Context, path string, headers map[string]string, response any) error {
	return c.request(ctx, http.MethodGet, path, headers, nil, response)
}

// Post 发送 POST 请求。
func (c *HTTPClient) Post(ctx context.Context, path string, headers map[string]string, body, response any) error {
	return c.request(ctx, http.MethodPost, path, headers, body, response)
}

// request 发送 HTTP 请求。
// path 可以是相对路径（如 "/api/token"）或完整 URL（如 "https://host.com/api/token"）。
// 如果 path 是完整 URL，则直接使用；否则与 baseURL 拼接。
func (c *HTTPClient) request(
	ctx context.Context,
	method, path string,
	headers map[string]string,
	body, response any,
) error {
	url := c.buildURL(path)

	// 开始 HTTP 请求观测
	// 使用 sanitizeURL 去除查询参数，避免高基数问题
	ctx, span := xmetrics.Start(ctx, c.observer, xmetrics.SpanOptions{
		Component: MetricsComponent,
		Operation: MetricsOpHTTPRequest,
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			{Key: MetricsAttrHTTPMethod, Value: method},
			{Key: MetricsAttrHTTPPath, Value: sanitizeURL(url)},
		},
	})
	var err error
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	bodyReader, err := c.buildRequestBody(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		err = fmt.Errorf("xauth: create request failed: %w", err)
		return err
	}

	c.setHeaders(req, headers)

	resp, err := c.client.Do(req)
	if err != nil {
		err = NewTemporaryError(fmt.Errorf("xauth: request failed: %w", err))
		return err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // Close 错误无法传播，通常可忽略

	err = c.handleResponse(resp, response)
	return err
}

// buildURL 构建请求 URL。
// 设计决策: 支持绝对 URL 是有意为之——同一认证域内可能需要跨主机请求。
// 调用方通过 AuthRequest.URL 传入，由调用方负责确保目标主机可信。
// baseURL 与 path 拼接时约定：baseURL 不含尾部斜杠，path 以斜杠开头。
func (c *HTTPClient) buildURL(path string) string {
	if isAbsoluteURL(path) {
		return path
	}
	return c.baseURL + path
}

// isAbsoluteURL 判断 path 是否为绝对 URL（大小写不敏感）。
// HTTP scheme 规范（RFC 3986 §3.1）要求 scheme 大小写不敏感。
func isAbsoluteURL(path string) bool {
	if len(path) >= 8 && strings.EqualFold(path[:8], "https://") {
		return true
	}
	return len(path) >= 7 && strings.EqualFold(path[:7], "http://")
}

// sanitizeURL 移除 URL 中的查询参数，避免观测指标高基数问题。
func sanitizeURL(rawURL string) string {
	if path, _, found := strings.Cut(rawURL, "?"); found {
		return path
	}
	return rawURL
}

// buildRequestBody 构建请求体。
// 支持以下类型：
//   - nil: 无请求体
//   - string: 直接作为请求体（用于 form-encoded 数据）
//   - []byte: 直接作为请求体
//   - io.Reader: 直接使用
//   - 其他: JSON 序列化
func (c *HTTPClient) buildRequestBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}
	switch v := body.(type) {
	case string:
		return strings.NewReader(v), nil
	case []byte:
		return bytes.NewReader(v), nil
	case io.Reader:
		return v, nil
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("xauth: marshal request body failed: %w", err)
		}
		return bytes.NewReader(data), nil
	}
}

// setHeaders 设置请求头。
// 设计决策: 仅在 headers 中未包含 Content-Type 时才设置默认值，
// 避免 JSON 默认值覆盖表单编码等其他内容类型。
func (c *HTTPClient) setHeaders(req *http.Request, headers map[string]string) {
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

// handleResponse 处理 HTTP 响应。
func (c *HTTPClient) handleResponse(resp *http.Response, response any) error {
	// 使用 LimitedReader 读取响应，多读取 1 字节用于检测截断
	lr := &io.LimitedReader{R: resp.Body, N: maxResponseSize + 1}
	respBody, err := io.ReadAll(lr)
	if err != nil {
		return fmt.Errorf("xauth: read response body failed: %w", err)
	}

	// 检测响应体是否被截断（读取了超过限制的数据）
	if len(respBody) > maxResponseSize {
		return fmt.Errorf("%w: limit %d bytes", ErrResponseTooLarge, maxResponseSize)
	}

	if resp.StatusCode >= 400 {
		return c.parseAPIError(resp.StatusCode, respBody)
	}

	if response != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, response); err != nil {
			return fmt.Errorf("xauth: unmarshal response failed: %w", err)
		}
	}

	return nil
}

// parseAPIError 解析 API 错误响应。
func (c *HTTPClient) parseAPIError(statusCode int, respBody []byte) error {
	var apiResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	// 解析失败不影响错误处理，apiResp 使用零值
	_ = json.Unmarshal(respBody, &apiResp) //nolint:errcheck // 解析失败使用零值即可
	return NewAPIError(statusCode, apiResp.Code, apiResp.Message)
}

// RequestWithAuth 发送带认证的请求。
// headers 会被克隆，不会修改调用方的原始 map。
func (c *HTTPClient) RequestWithAuth(
	ctx context.Context,
	method, path string,
	token string,
	headers map[string]string,
	body, response any,
) error {
	h := make(map[string]string, len(headers)+1)
	maps.Copy(h, headers)
	h["Authorization"] = "Bearer " + token
	return c.request(ctx, method, path, h, body, response)
}

// Client 返回底层 HTTP 客户端。
func (c *HTTPClient) Client() *http.Client {
	return c.client
}
