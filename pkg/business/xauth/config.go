package xauth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// =============================================================================
// 默认值
// =============================================================================

const (
	// DefaultTimeout 默认请求超时时间。
	DefaultTimeout = 15 * time.Second

	// DefaultTokenRefreshThreshold Token 刷新阈值。
	// Token 剩余有效期小于此值时触发后台刷新。
	DefaultTokenRefreshThreshold = 5 * time.Minute

	// DefaultPlatformDataCacheTTL 平台数据缓存 TTL。
	DefaultPlatformDataCacheTTL = 30 * time.Minute

	// DefaultTokenCacheTTL Token 缓存 TTL。
	// 实际 TTL 会根据 Token 过期时间动态计算。
	DefaultTokenCacheTTL = 6 * time.Hour

	// DefaultLocalClientID 本地环境默认客户端 ID。
	DefaultLocalClientID = "local"

	// DefaultSaaSClientID SaaS 环境默认客户端 ID。
	DefaultSaaSClientID = "saas"
)

// =============================================================================
// 环境变量 Key
// =============================================================================

const (
	// EnvKeyTenantID 租户 ID 环境变量。
	EnvKeyTenantID = "TENANT_PROJECT_ID"

	// EnvKeyDeploymentType 部署类型环境变量。
	EnvKeyDeploymentType = "DEPLOYMENT_TYPE"

	// DeploymentTypeLocal 本地部署类型。
	DeploymentTypeLocal = "LOCAL"

	// DeploymentTypeSaaS SaaS 部署类型。
	DeploymentTypeSaaS = "SAAS"
)

// =============================================================================
// API 路由（示例默认值）
// =============================================================================

// 设计决策: 路径与配置分离——下列常量只是开源版本的示例路由，用于 DefaultPaths()。
// 实际部署的认证服务路由通过 Config.Paths 覆盖，代码中不保留任何环境专属的值。

//nolint:gosec // G101: 这些是 API 路径常量，不是凭据
const (
	// PathTokenVerify Token 验证路径。
	PathTokenVerify = "/auth/api/v1/oauth/token/verify"

	// PathTokenObtain Token 获取路径（client_credentials）。
	PathTokenObtain = "/auth/oauth/token"

	// PathAPIAccessToken API Key 获取 Token 路径。
	PathAPIAccessToken = "/auth/api/v1/apiAccessTokens/login"

	// PathPlatformSelf 获取当前平台信息路径。
	PathPlatformSelf = "/auth/api/v1/platforms/self"

	// PathUnclassRegion 获取未归类组 Region 路径。
	PathUnclassRegion = "/auth/api/v1/regions/uncategorized"

	// PathHasParent 判断是否有父平台路径。
	PathHasParent = "/auth/api/v1/platforms/hasSuper"
)

// Paths 认证服务各接口的路径。
// 零值字段在 ApplyDefaults 时填充为 DefaultPaths() 返回的示例路径。
type Paths struct {
	// TokenVerify Token 验证路径。
	TokenVerify string
	// TokenObtain Token 获取路径（client_credentials）。
	TokenObtain string
	// APIAccessToken API Key 获取 Token 路径。
	APIAccessToken string
	// PlatformSelf 获取当前平台信息路径。
	PlatformSelf string
	// UnclassRegion 获取未归类组 Region 路径。
	UnclassRegion string
	// HasParent 判断是否有父平台路径。
	HasParent string
}

// DefaultPaths 返回示例路径集合。
func DefaultPaths() Paths {
	return Paths{
		TokenVerify:    PathTokenVerify,
		TokenObtain:    PathTokenObtain,
		APIAccessToken: PathAPIAccessToken,
		PlatformSelf:   PathPlatformSelf,
		UnclassRegion:  PathUnclassRegion,
		HasParent:      PathHasParent,
	}
}

// applyDefaults 为空字段填充示例路径。
func (p *Paths) applyDefaults() {
	d := DefaultPaths()
	if p.TokenVerify == "" {
		p.TokenVerify = d.TokenVerify
	}
	if p.TokenObtain == "" {
		p.TokenObtain = d.TokenObtain
	}
	if p.APIAccessToken == "" {
		p.APIAccessToken = d.APIAccessToken
	}
	if p.PlatformSelf == "" {
		p.PlatformSelf = d.PlatformSelf
	}
	if p.UnclassRegion == "" {
		p.UnclassRegion = d.UnclassRegion
	}
	if p.HasParent == "" {
		p.HasParent = d.HasParent
	}
}

// validate 校验已设置的路径必须以 "/" 开头（空值由 applyDefaults 填充，不在此校验）。
func (p *Paths) validate() error {
	for _, v := range []string{p.TokenVerify, p.TokenObtain, p.APIAccessToken, p.PlatformSelf, p.UnclassRegion, p.HasParent} {
		if v != "" && !strings.HasPrefix(v, "/") {
			return fmt.Errorf("%w: %q", ErrInvalidPath, v)
		}
	}
	return nil
}

// =============================================================================
// 缓存字段
// =============================================================================

const (
	// CacheFieldPlatformID 平台 ID 缓存字段。
	CacheFieldPlatformID = "platform_id"

	// CacheFieldUnclassRegionID 未归类组 Region ID 缓存字段。
	CacheFieldUnclassRegionID = "unclass_region_id"

	// CacheFieldHasParent 是否有父平台缓存字段。
	CacheFieldHasParent = "has_parent"
)

// =============================================================================
// Config 配置结构
// =============================================================================

// Config 定义 xauth 客户端配置。
type Config struct {
	// Host 认证服务地址（必填）。
	// 必须使用 https:// 前缀，除非显式设置 AllowInsecure = true。
	// 例如：https://auth.example.com
	Host string

	// AllowInsecure 允许使用 http:// 非加密连接。
	// 设计决策: 默认强制 HTTPS——认证服务传输 Bearer Token 和客户端凭据，
	// 明文 HTTP 会暴露这些敏感信息。仅在开发/测试环境中启用此选项。
	AllowInsecure bool

	// Paths 认证服务各接口的路径。
	// 零值字段在 ApplyDefaults 时填充为 DefaultPaths() 的示例路径；
	// 示例值仅用于开源版本，实际部署请按认证服务的真实路由覆盖。
	Paths Paths

	// ClientID 客户端 ID。
	// 为空时根据 DEPLOYMENT_TYPE 自动选择：
	//   - LOCAL: "local"
	//   - 其他: "saas"
	ClientID string

	// ClientSecret 客户端密钥。
	// 使用 client_credentials 方式获取 Token 时需要。
	ClientSecret string

	// APIKey API Key（可选）。
	// 如果设置，优先使用 API Key 方式获取 Token。
	APIKey string

	// Timeout 请求超时时间。
	// 默认 15 秒。
	Timeout time.Duration

	// TokenRefreshThreshold Token 刷新阈值。
	// Token 剩余有效期小于此值时触发后台刷新。
	// 默认 5 分钟。
	TokenRefreshThreshold time.Duration

	// PlatformDataCacheTTL 平台数据缓存 TTL。
	// 默认 30 分钟。
	PlatformDataCacheTTL time.Duration

	// TLS TLS 配置。
	// 为 nil 时使用默认配置（启用证书验证）。
	// 开发/测试环境可设置 InsecureSkipVerify: true 跳过证书验证。
	TLS *TLSConfig
}

// TLSConfig TLS 配置。
type TLSConfig struct {
	// InsecureSkipVerify 是否跳过证书验证。
	// 仅用于开发/测试环境，生产环境请勿启用。
	InsecureSkipVerify bool

	// RootCAFile CA 证书文件路径。
	RootCAFile string

	// CertFile 客户端证书文件路径。
	CertFile string

	// KeyFile 客户端密钥文件路径。
	KeyFile string
}

// Validate 验证配置有效性。
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}

	if err := c.validateHost(); err != nil {
		return err
	}

	if c.Timeout < 0 {
		return ErrInvalidTimeout
	}

	if c.TokenRefreshThreshold < 0 {
		return ErrInvalidRefreshThreshold
	}

	return c.Paths.validate()
}

// validateHost 校验 Host 格式和协议安全性。
func (c *Config) validateHost() error {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return ErrMissingHost
	}

	// 设计决策: 使用 net/url 严格校验 Host 格式，确保包含有效的 scheme 和主机名。
	// 无 scheme 的地址（如 "auth.example.com"）在拼接 API 路径后无法正确请求，
	// 通过 fail-fast 在配置阶段暴露问题，而非在运行期请求失败。
	u, err := url.Parse(host)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrInvalidHost
	}

	// 设计决策: 强制 HTTPS——认证服务传输 Bearer Token 和客户端凭据，
	// 明文 HTTP 会将凭据暴露给网络上的窃听者。
	// 开发/测试环境可通过 AllowInsecure = true 放行 http://。
	if !c.AllowInsecure && u.Scheme != "https" {
		return ErrInsecureHost
	}

	return nil
}

// ApplyDefaults 应用默认值。
func (c *Config) ApplyDefaults() {
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}

	if c.TokenRefreshThreshold == 0 {
		c.TokenRefreshThreshold = DefaultTokenRefreshThreshold
	}

	if c.PlatformDataCacheTTL == 0 {
		c.PlatformDataCacheTTL = DefaultPlatformDataCacheTTL
	}

	if c.ClientID == "" {
		c.ClientID = getDefaultClientID()
	}

	c.Paths.applyDefaults()

	// 设计决策: ClientSecret 默认与 ClientID 相同，这是认证服务的约定——
	// 内部 client_credentials 模式下 secret 与 id 一致，简化配置。
	// 外部调用方如需独立 secret，通过 Config.ClientSecret 显式指定。
	if c.ClientSecret == "" {
		c.ClientSecret = c.ClientID
	}
}

// Clone 创建配置的深拷贝。
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}

	clone := *c
	if c.TLS != nil {
		tlsCopy := *c.TLS
		clone.TLS = &tlsCopy
	}
	return &clone
}

// BuildTLSConfig 构建 TLS 配置。
func (c *TLSConfig) BuildTLSConfig() (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}

	//nolint:gosec // G402: InsecureSkipVerify 由用户配置控制，doc.go 中有安全警告
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	// 加载 CA 证书
	if c.RootCAFile != "" {
		caCert, err := os.ReadFile(c.RootCAFile)
		if err != nil {
			return nil, fmt.Errorf("xauth: failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("xauth: failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// 加载客户端证书
	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("xauth: failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// getDefaultClientID 根据部署类型获取默认客户端 ID。
func getDefaultClientID() string {
	if IsLocalEnv() {
		return DefaultLocalClientID
	}
	return DefaultSaaSClientID
}

// IsLocalEnv 判断是否为本地环境。
func IsLocalEnv() bool {
	return os.Getenv(EnvKeyDeploymentType) == DeploymentTypeLocal
}

// GetTenantIDFromEnv 从环境变量获取租户 ID。
func GetTenantIDFromEnv() string {
	return os.Getenv(EnvKeyTenantID)
}
