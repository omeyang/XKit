package xdlock

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/storage/xetcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// =============================================================================
// etcd 客户端配置
// =============================================================================

// EtcdConfig etcd 客户端配置。
// 这是 xetcd.Config 的类型别名，支持 JSON/YAML 反序列化。
type EtcdConfig = xetcd.Config

// DefaultEtcdConfig 返回默认配置。
//
// 默认值：
//   - DialTimeout: 5 秒
//   - DialKeepAliveTime: 10 秒
//   - DialKeepAliveTimeout: 3 秒
//   - RejectOldCluster: true
//   - PermitWithoutStream: true
func DefaultEtcdConfig() *EtcdConfig {
	return xetcd.DefaultConfig()
}

// =============================================================================
// etcd 客户端选项
// =============================================================================

// etcdClientOptions 内部选项结构。
type etcdClientOptions struct {
	Context       context.Context
	HealthCheck   bool
	HealthTimeout time.Duration
	TLSConfig     *tls.Config
}

// defaultEtcdClientOptions 返回默认选项。
func defaultEtcdClientOptions() *etcdClientOptions {
	return &etcdClientOptions{
		Context:       context.Background(),
		HealthCheck:   false,
		HealthTimeout: 10 * time.Second,
	}
}

// EtcdClientOption 定义 etcd 客户端的配置选项。
type EtcdClientOption func(*etcdClientOptions)

// WithEtcdClientContext 设置客户端上下文，用于控制客户端生命周期。
func WithEtcdClientContext(ctx context.Context) EtcdClientOption {
	return func(o *etcdClientOptions) {
		if ctx != nil {
			o.Context = ctx
		}
	}
}

// WithEtcdHealthCheck 创建后执行健康检查。
// enabled 为 true 时，创建客户端后执行一次 Get 操作验证连接。
// timeout 为健康检查超时时间，默认 10 秒。
func WithEtcdHealthCheck(enabled bool, timeout time.Duration) EtcdClientOption {
	return func(o *etcdClientOptions) {
		o.HealthCheck = enabled
		if timeout > 0 {
			o.HealthTimeout = timeout
		}
	}
}

// WithEtcdTLS 设置 TLS 配置，用于启用安全连接。
func WithEtcdTLS(config *tls.Config) EtcdClientOption {
	return func(o *etcdClientOptions) {
		o.TLSConfig = config
	}
}

// =============================================================================
// etcd 客户端创建
// =============================================================================

// NewEtcdClient 根据配置创建 etcd 客户端。
// 返回原始的 *clientv3.Client，如需更丰富的 KV 操作，请使用 xetcd.NewClient()。
//
// 错误：ErrNilConfig、ErrNoEndpoints、连接错误、健康检查错误。
func NewEtcdClient(config *EtcdConfig, opts ...EtcdClientOption) (*clientv3.Client, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	if err := config.Validate(); err != nil {
		return nil, convertXetcdError(err)
	}

	options := defaultEtcdClientOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 构建 xetcd 选项
	var xetcdOpts []xetcd.Option
	if options.Context != nil {
		xetcdOpts = append(xetcdOpts, xetcd.WithContext(options.Context))
	}
	if options.HealthCheck {
		xetcdOpts = append(xetcdOpts, xetcd.WithHealthCheck(true, options.HealthTimeout))
	}
	if options.TLSConfig != nil {
		xetcdOpts = append(xetcdOpts, xetcd.WithTLS(options.TLSConfig))
	}

	// 使用 xetcd 创建客户端
	client, err := xetcd.NewClient(config, xetcdOpts...)
	if err != nil {
		return nil, convertXetcdError(err)
	}

	return client.RawClient(), nil
}

// =============================================================================
// 便捷函数
// =============================================================================

// NewEtcdFactoryFromConfig 从配置创建 etcd 锁工厂。
// 便捷函数，等同于 NewEtcdClient + NewEtcdFactory。
// 返回的 client 需要调用方负责关闭。
func NewEtcdFactoryFromConfig(
	config *EtcdConfig,
	clientOpts []EtcdClientOption,
	factoryOpts ...EtcdFactoryOption,
) (EtcdFactory, *clientv3.Client, error) {
	client, err := NewEtcdClient(config, clientOpts...)
	if err != nil {
		return nil, nil, err
	}

	factory, err := NewEtcdFactory(client, factoryOpts...)
	if err != nil {
		closeErr := client.Close()
		return nil, nil, errors.Join(err, closeErr)
	}

	return factory, client, nil
}

// =============================================================================
// 内部函数
// =============================================================================

// convertXetcdError 将 xetcd 错误转换为 xdlock 错误。
func convertXetcdError(err error) error {
	if err == nil {
		return nil
	}
	switch err {
	case xetcd.ErrNilConfig:
		return ErrNilConfig
	case xetcd.ErrNoEndpoints:
		return ErrNoEndpoints
	default:
		return fmt.Errorf("xdlock: %w", err)
	}
}
