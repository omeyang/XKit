package xetcd

import (
	"context"
	"testing"
)

func TestNewClient_NilConfig(t *testing.T) {
	_, err := NewClient(nil)
	if err != ErrNilConfig {
		t.Errorf("NewClient(nil) error = %v, want %v", err, ErrNilConfig)
	}
}

func TestNewClient_NoEndpoints(t *testing.T) {
	_, err := NewClient(&Config{})
	if err != ErrNoEndpoints {
		t.Errorf("NewClient with empty endpoints error = %v, want %v", err, ErrNoEndpoints)
	}
}

func TestNewClient_InvalidEndpoint(t *testing.T) {
	// 无效端点应该在连接时失败
	// 由于没有真实的 etcd 服务，这个测试主要验证代码路径
	config := &Config{
		Endpoints: []string{"invalid:9999"},
	}

	// 不启用健康检查时，创建客户端可能成功（延迟连接）
	// 但由于我们使用的是真实的 etcd 客户端，连接失败可能延迟到首次操作
	client, err := NewClient(config)
	if err == nil && client != nil {
		// 如果创建成功，确保能正确关闭
		_ = client.Close(context.Background())
	}
	// 不检查具体错误，因为行为取决于 etcd 客户端实现
}

func TestNewClient_NilOption(t *testing.T) {
	config := &Config{Endpoints: []string{"localhost:2379"}}
	_, err := NewClient(config, nil)
	if err != ErrNilOption {
		t.Errorf("NewClient with nil option error = %v, want %v", err, ErrNilOption)
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	// 创建一个模拟场景：验证 Close 的幂等性
	// 由于需要真实 etcd，这里只验证逻辑

	// 模拟 closed 状态
	c := &Client{}
	c.closed.Store(true)

	// 第二次关闭应该直接返回 nil
	err := c.Close(context.Background())
	if err != nil {
		t.Errorf("Close() on already closed client should return nil, got %v", err)
	}
}

func TestClient_Close_ZeroValue(t *testing.T) {
	// 零值 Client（未通过 NewClient 创建）调用 Close 不应 panic
	c := &Client{}
	err := c.Close(context.Background())
	if err != nil {
		t.Errorf("Close() on zero-value client should return nil, got %v", err)
	}
}

func TestClient_checkClosed(t *testing.T) {
	c := &Client{}

	// 未关闭时
	if err := c.checkClosed(); err != nil {
		t.Errorf("checkClosed() on open client = %v, want nil", err)
	}

	// 关闭后
	c.closed.Store(true)
	if err := c.checkClosed(); err != ErrClientClosed {
		t.Errorf("checkClosed() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestClient_checkPreconditions(t *testing.T) {
	// nil context（优先检查）
	c := &Client{}
	if err := c.checkPreconditions(nil); err != ErrNilContext { //nolint:staticcheck // 测试 nil ctx 防御
		t.Errorf("checkPreconditions(nil) = %v, want %v", err, ErrNilContext)
	}

	// 零值 Client（client 字段为 nil）
	if err := c.checkPreconditions(context.Background()); err != ErrNotInitialized {
		t.Errorf("checkPreconditions(ctx) on zero-value client = %v, want %v", err, ErrNotInitialized)
	}

	// 正常初始化的 Client
	c2 := newTestClientForPreconditions()
	if err := c2.checkPreconditions(context.Background()); err != nil {
		t.Errorf("checkPreconditions(ctx) on initialized client = %v, want nil", err)
	}

	// 已关闭的 Client
	c2.closed.Store(true)
	if err := c2.checkPreconditions(context.Background()); err != ErrClientClosed {
		t.Errorf("checkPreconditions(ctx) on closed client = %v, want %v", err, ErrClientClosed)
	}
}

// newTestClientForPreconditions 创建用于前置条件测试的最小 Client。
func newTestClientForPreconditions() *Client {
	return &Client{
		client:  &noopEtcdClient{},
		closeCh: make(chan struct{}),
	}
}

// TestClient_ZeroValue_NoPanic 验证零值 Client 调用所有公开方法不会 panic。
func TestClient_ZeroValue_NoPanic(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	if _, err := c.Get(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("Get() = %v, want %v", err, ErrNotInitialized)
	}
	if _, _, err := c.GetWithRevision(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("GetWithRevision() = %v, want %v", err, ErrNotInitialized)
	}
	if err := c.Put(ctx, "key", []byte("v")); err != ErrNotInitialized {
		t.Errorf("Put() = %v, want %v", err, ErrNotInitialized)
	}
	if err := c.PutWithTTL(ctx, "key", []byte("v"), 10); err != ErrNotInitialized {
		t.Errorf("PutWithTTL() = %v, want %v", err, ErrNotInitialized)
	}
	if err := c.Delete(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("Delete() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.DeleteWithPrefix(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("DeleteWithPrefix() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.List(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("List() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.ListKeys(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("ListKeys() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.Exists(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("Exists() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.Count(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("Count() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.Watch(ctx, "key"); err != ErrNotInitialized {
		t.Errorf("Watch() = %v, want %v", err, ErrNotInitialized)
	}
	if _, err := c.WatchWithRetry(ctx, "key", DefaultRetryConfig()); err != ErrNotInitialized {
		t.Errorf("WatchWithRetry() = %v, want %v", err, ErrNotInitialized)
	}
}

func TestClient_isClosed(t *testing.T) {
	c := &Client{}

	if c.isClosed() {
		t.Error("isClosed() should return false for new client")
	}

	c.closed.Store(true)
	if !c.isClosed() {
		t.Error("isClosed() should return true after Close")
	}
}
