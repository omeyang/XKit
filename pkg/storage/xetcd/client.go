package xetcd

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// Client etcd 客户端封装。
// 提供简化的 KV 操作和 Watch 功能。
//
// Client 是并发安全的，可以被多个 goroutine 同时使用。
type Client struct {
	client    etcdClient // 使用接口以支持测试时的 mock 注入
	rawClient *clientv3.Client
	config    *Config
	closed    atomic.Bool
}

// NewClient 创建 etcd 客户端。
//
// 参数：
//   - config: etcd 配置，至少需要 Endpoints
//   - opts: 可选的客户端选项
//
// 返回：
//   - *Client: etcd 客户端封装
//   - error: 创建失败时返回错误
//
// 错误：
//   - ErrNilConfig: config 为 nil
//   - ErrNoEndpoints: 未配置 endpoints
//   - 连接错误: etcd 连接失败
//   - 健康检查错误: 启用健康检查但检查失败
func NewClient(config *Config, opts ...Option) (*Client, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 应用选项
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	// 应用配置默认值
	cfg := config.applyDefaults()

	// 构建 clientv3.Config
	clientConfig := clientv3.Config{
		Endpoints:            cfg.Endpoints,
		DialTimeout:          cfg.DialTimeout,
		DialKeepAliveTime:    cfg.DialKeepAliveTime,
		DialKeepAliveTimeout: cfg.DialKeepAliveTimeout,
		Username:             cfg.Username,
		Password:             cfg.Password,
		AutoSyncInterval:     cfg.AutoSyncInterval,
		RejectOldCluster:     cfg.RejectOldCluster,
		DialOptions: []grpc.DialOption{
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                cfg.DialKeepAliveTime,
				Timeout:             cfg.DialKeepAliveTimeout,
				PermitWithoutStream: cfg.PermitWithoutStream,
			}),
		},
	}

	// TLS 配置
	if o.tlsConfig != nil {
		clientConfig.TLS = o.tlsConfig
	}

	// 创建客户端
	rawClient, err := clientv3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("xetcd: create client: %w", err)
	}

	// 可选健康检查
	if o.healthCheck {
		ctx, cancel := context.WithTimeout(o.ctx, o.healthTimeout)
		defer cancel()
		if _, err := rawClient.Get(ctx, "xetcd-health-check"); err != nil {
			closeErr := rawClient.Close()
			return nil, errors.Join(
				fmt.Errorf("xetcd: health check failed: %w", err),
				closeErr,
			)
		}
	}

	return &Client{
		client:    rawClient,
		rawClient: rawClient,
		config:    cfg,
	}, nil
}

// RawClient 返回原生 etcd 客户端。
// 用于需要直接操作原生 API 的场景，如事务、租约等高级操作。
func (c *Client) RawClient() *clientv3.Client {
	return c.rawClient
}

// Close 关闭客户端连接。
// 关闭后客户端不可再使用。
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil // 已经关闭
	}
	return c.client.Close()
}

// isClosed 检查客户端是否已关闭。
func (c *Client) isClosed() bool {
	return c.closed.Load()
}

// checkClosed 检查客户端状态，如已关闭返回错误。
func (c *Client) checkClosed() error {
	if c.isClosed() {
		return ErrClientClosed
	}
	return nil
}
