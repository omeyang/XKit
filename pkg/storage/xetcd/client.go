package xetcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	config    *Config // 保留已规范化的配置副本，用于调试和未来扩展（如 Reconnect、config 审计）
	closed    atomic.Bool
	closeCh   chan struct{}  // 关闭信号通道，用于通知 Watch goroutine 退出
	watchWg   sync.WaitGroup // 追踪活跃的 Watch goroutine，确保 Close 时等待退出
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
//
// 测试说明：NewClient 依赖 clientv3.New 建立真实 gRPC 连接，
// 健康检查、TLS 和 errors.Join 路径仅在集成测试中覆盖（见 integration_test.go）。
// 单元测试覆盖率约 65%，为该函数的已知限制。
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
		if opt == nil {
			return nil, ErrNilOption
		}
		opt(o)
	}

	// 应用配置默认值
	cfg := config.applyDefaults()

	// 构建 clientv3.Config
	// 设计决策: keepalive 参数仅通过 DialOptions 设置，不同时设置 Config 字段。
	// etcd 客户端内部会将 Config.DialKeepAliveTime/Timeout 转换为 gRPC DialOption，
	// 与显式 DialOptions 合并时后者覆盖前者。去除冗余避免两处值不一致的隐患，
	// 且显式 DialOptions 能控制 PermitWithoutStream 字段。
	clientConfig := clientv3.Config{
		Endpoints:        cfg.Endpoints,
		DialTimeout:      cfg.DialTimeout,
		Username:         cfg.Username,
		Password:         cfg.Password,
		AutoSyncInterval: cfg.AutoSyncInterval,
		RejectOldCluster: cfg.RejectOldCluster,
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
		if _, err := rawClient.Get(ctx, o.healthCheckKey); err != nil {
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
		closeCh:   make(chan struct{}),
	}, nil
}

// RawClient 返回原生 etcd 客户端。
// 设计决策: 暴露原生客户端用于事务（Txn）、租约续约（KeepAlive）等高级操作，
// 这些功能不在 xetcd 的简化封装范围内。参见 doc.go 中的"设计边界"说明。
// 设计决策: 不检查 closed 状态，因为返回 (*clientv3.Client, error) 会破坏 API 兼容性，
// 且底层 etcd 客户端在 Close() 后操作自然会返回错误，无需额外保护。
func (c *Client) RawClient() *clientv3.Client {
	return c.rawClient
}

// Close 关闭客户端连接并通知所有 Watch goroutine 退出。
// 关闭后客户端不可再使用。
// Client 必须通过 NewClient 创建，零值 Client 调用公开方法会返回 ErrNotInitialized。
//
// Close 会等待所有 Watch goroutine 退出后再关闭底层连接，
// 确保关闭后不会有残留的 goroutine 继续使用已关闭的连接。
//
// 设计决策: ctx 参数当前未使用，保留是为了与 D-02 决策保持一致
// （统一生命周期接口保留 ctx 参数以保持扩展性），
// 未来可用于控制关闭超时（如等待 Watch goroutine 退出）。
// nil ctx 不会导致错误，内部会替换为 context.Background()。
func (c *Client) Close(_ context.Context) error {
	if c.closed.Swap(true) {
		return nil // 已经关闭
	}
	if c.closeCh != nil {
		close(c.closeCh)
	}
	// 设计决策: 先等待所有 Watch goroutine 退出，再关闭底层连接。
	// 这确保 goroutine 不会在关闭后继续使用已关闭的 etcd 客户端，
	// 避免高频创建/销毁场景下的资源泄漏与停机抖动。
	c.watchWg.Wait()
	if c.client != nil {
		return c.client.Close()
	}
	return nil
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

// checkPreconditions 检查公开方法的前置条件：nil ctx、未初始化和 closed 状态。
// 所有接受 context.Context 的公开方法应在入口处调用此方法，
// 避免 nil ctx 传递到 etcd 客户端导致 panic，
// 以及零值 Client 访问 nil client 字段导致 panic。
func (c *Client) checkPreconditions(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if c.client == nil {
		return ErrNotInitialized
	}
	return c.checkClosed()
}
