package xetcd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config etcd 客户端配置。
// 支持 JSON/YAML 反序列化。
//
// 推荐使用 DefaultConfig() 获取带有推荐默认值的配置，然后按需覆盖：
//
//	cfg := xetcd.DefaultConfig()
//	cfg.Endpoints = []string{"localhost:2379"}
//	client, err := xetcd.NewClient(cfg)
type Config struct {
	// Endpoints etcd 服务端点列表，必填。
	// 格式：["host1:port1", "host2:port2"]
	Endpoints []string `json:"endpoints" yaml:"endpoints"`

	// Username 用户名（可选）。
	// 启用认证时需要配置。
	Username string `json:"username" yaml:"username"`

	// Password 密码（可选）。
	// 启用认证时需要配置。
	Password string `json:"password" yaml:"password"`

	// DialTimeout 连接超时。
	// 零值时使用默认值 5 秒。
	DialTimeout time.Duration `json:"dialTimeout" yaml:"dialTimeout"`

	// DialKeepAliveTime gRPC keepalive 探测间隔。
	// 零值时使用默认值 10 秒。
	// 连接空闲超过此时间后发送 keepalive 探测。
	DialKeepAliveTime time.Duration `json:"dialKeepAliveTime" yaml:"dialKeepAliveTime"`

	// DialKeepAliveTimeout gRPC keepalive 超时。
	// 零值时使用默认值 3 秒。
	// keepalive 探测的最大等待时间。
	DialKeepAliveTimeout time.Duration `json:"dialKeepAliveTimeout" yaml:"dialKeepAliveTimeout"`

	// AutoSyncInterval 自动同步 endpoints 间隔，默认 0（禁用）。
	// 设置后会定期从集群获取最新的 endpoints 列表。
	AutoSyncInterval time.Duration `json:"autoSyncInterval" yaml:"autoSyncInterval"`

	// RejectOldCluster 拒绝过期集群。
	// 设置为 true 时，如果集群版本过低会拒绝连接。
	//
	// 注意：由于 Go 布尔零值为 false，直接使用 Config{} 时此字段为 false。
	// 推荐使用 DefaultConfig() 获取安全的默认配置（true）。
	RejectOldCluster bool `json:"rejectOldCluster" yaml:"rejectOldCluster"`

	// PermitWithoutStream 允许无流的 keepalive。
	// 设置为 true 时，即使没有活跃的 RPC 流也会发送 keepalive。
	//
	// 注意：由于 Go 布尔零值为 false，直接使用 Config{} 时此字段为 false。
	// 推荐使用 DefaultConfig() 获取推荐配置（true）。
	PermitWithoutStream bool `json:"permitWithoutStream" yaml:"permitWithoutStream"`
}

// 默认配置值。
const (
	defaultDialTimeout          = 5 * time.Second
	defaultDialKeepAliveTime    = 10 * time.Second
	defaultDialKeepAliveTimeout = 3 * time.Second
)

// DefaultConfig 返回带有推荐默认值的配置。
//
// 推荐使用此函数创建配置，然后按需覆盖字段，而不是直接使用 Config{}。
// 这样可以确保布尔字段（RejectOldCluster、PermitWithoutStream）使用安全的默认值。
//
// 默认值：
//   - DialTimeout: 5 秒
//   - DialKeepAliveTime: 10 秒
//   - DialKeepAliveTimeout: 3 秒
//   - RejectOldCluster: true（安全默认值，拒绝过期集群）
//   - PermitWithoutStream: true（保持连接健康）
//
// 示例：
//
//	cfg := xetcd.DefaultConfig()
//	cfg.Endpoints = []string{"localhost:2379"}
//	cfg.Username = "admin"
//	cfg.Password = "secret"
//	client, err := xetcd.NewClient(cfg)
func DefaultConfig() *Config {
	return &Config{
		DialTimeout:          defaultDialTimeout,
		DialKeepAliveTime:    defaultDialKeepAliveTime,
		DialKeepAliveTimeout: defaultDialKeepAliveTimeout,
		RejectOldCluster:     true,
		PermitWithoutStream:  true,
	}
}

// String 返回配置的字符串表示，密码字段会被遮蔽。
// 避免在日志、错误信息或调试输出中泄露敏感信息。
func (c *Config) String() string {
	password := ""
	if c.Password != "" {
		password = "***"
	}
	return fmt.Sprintf("Config{Endpoints:%v Username:%s Password:%s DialTimeout:%v "+
		"DialKeepAliveTime:%v DialKeepAliveTimeout:%v AutoSyncInterval:%v "+
		"RejectOldCluster:%v PermitWithoutStream:%v}",
		c.Endpoints, c.Username, password, c.DialTimeout,
		c.DialKeepAliveTime, c.DialKeepAliveTimeout, c.AutoSyncInterval,
		c.RejectOldCluster, c.PermitWithoutStream)
}

// Validate 验证配置有效性。
// 检查必填字段是否已配置，并验证 endpoint 格式。
//
// 有效的 endpoint 格式为 "host:port"，例如：
//   - "localhost:2379"
//   - "192.168.1.1:2379"
//   - "etcd.example.com:2379"
//   - "[::1]:2379"（IPv6 格式）
func (c *Config) Validate() error {
	if len(c.Endpoints) == 0 {
		return ErrNoEndpoints
	}

	// 验证每个 endpoint 的格式
	for i, ep := range c.Endpoints {
		if ep == "" {
			return fmt.Errorf("%w: endpoint[%d] is empty", ErrInvalidEndpoint, i)
		}
		if err := validateEndpoint(ep); err != nil {
			return fmt.Errorf("%w: endpoint[%d]=%q %v", ErrInvalidEndpoint, i, ep, err)
		}
	}

	// 验证 Duration 字段非负（零值由 applyDefaults 补充默认值，但负值是无效配置）
	if c.DialTimeout < 0 {
		return fmt.Errorf("%w: DialTimeout must not be negative", ErrInvalidConfig)
	}
	if c.DialKeepAliveTime < 0 {
		return fmt.Errorf("%w: DialKeepAliveTime must not be negative", ErrInvalidConfig)
	}
	if c.DialKeepAliveTimeout < 0 {
		return fmt.Errorf("%w: DialKeepAliveTimeout must not be negative", ErrInvalidConfig)
	}
	if c.AutoSyncInterval < 0 {
		return fmt.Errorf("%w: AutoSyncInterval must not be negative", ErrInvalidConfig)
	}

	return nil
}

// validateEndpoint 验证单个 endpoint 的格式。
// 支持 IPv4、IPv6 和域名格式，必须包含有效端口。
func validateEndpoint(ep string) error {
	endpoint := ep

	// 1. 去除可能的 scheme 前缀（如 http://、https://）
	if idx := strings.Index(ep, "://"); idx != -1 {
		endpoint = ep[idx+3:]
	}

	// 2. 处理 IPv6 格式 [host]:port
	if strings.HasPrefix(endpoint, "[") {
		// 必须有 ]:port 格式
		idx := strings.LastIndex(endpoint, "]:")
		if idx == -1 {
			return fmt.Errorf("invalid IPv6 endpoint format, expected [host]:port")
		}
		portPart := endpoint[idx+2:]
		if portPart == "" {
			return fmt.Errorf("missing port after IPv6 address")
		}
		if !isValidPort(portPart) {
			return fmt.Errorf("invalid port %q", portPart)
		}
		return nil
	}

	// 3. 处理 IPv4/hostname 格式 host:port
	lastColon := strings.LastIndex(endpoint, ":")
	if lastColon == -1 {
		return fmt.Errorf("missing port")
	}
	host := endpoint[:lastColon]
	if host == "" {
		return fmt.Errorf("missing host")
	}
	portPart := endpoint[lastColon+1:]
	if portPart == "" {
		return fmt.Errorf("missing port")
	}
	if !isValidPort(portPart) {
		return fmt.Errorf("invalid port %q", portPart)
	}
	return nil
}

// isValidPort 检查端口号是否有效（1-65535）。
func isValidPort(s string) bool {
	port, err := strconv.Atoi(s)
	return err == nil && port > 0 && port <= 65535
}

// applyDefaults 应用默认值，返回新的配置（不修改原配置）。
func (c *Config) applyDefaults() *Config {
	cfg := *c // 复制，避免修改原配置
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	if cfg.DialKeepAliveTime == 0 {
		cfg.DialKeepAliveTime = defaultDialKeepAliveTime
	}
	if cfg.DialKeepAliveTimeout == 0 {
		cfg.DialKeepAliveTimeout = defaultDialKeepAliveTimeout
	}
	return &cfg
}
