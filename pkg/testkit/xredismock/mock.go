package xredismock

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Mock 封装 miniredis 提供可控的 Redis 测试实例。
// 适用场景：xcache / xdlock(Redis) / xlimit 等需要 Redis 后端的单元测试。
// 并发安全：Close 幂等。
type Mock struct {
	server *miniredis.Miniredis
	client *redis.Client

	mu     sync.Mutex
	closed bool
}

// New 创建一个启动好的 Redis mock。失败时返回错误。
func New() (*Mock, error) {
	server, err := miniredis.Run()
	if err != nil {
		return nil, fmt.Errorf("xredismock: start miniredis: %w", err)
	}
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	return &Mock{server: server, client: client}, nil
}

// Addr 返回 mock 监听地址。
func (m *Mock) Addr() string { return m.server.Addr() }

// Client 返回连接到 mock 的 *redis.Client。
func (m *Mock) Client() *redis.Client { return m.client }

// Server 返回底层 *miniredis.Miniredis，用于高阶断言（FastForward 时间、检查 key 等）。
func (m *Mock) Server() *miniredis.Miniredis { return m.server }

// Close 关闭客户端和 mock 服务器；幂等。
// 客户端关闭错误仅记录日志，清理上下文无传播渠道。
func (m *Mock) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true

	if m.client != nil {
		if err := m.client.Close(); err != nil {
			slog.Warn("xredismock: close client", "err", err)
		}
	}
	if m.server != nil {
		m.server.Close()
	}
}

// NewClient 便捷构造：返回 *redis.Client 和清理函数。
// 适合 `client, cleanup, err := xredismock.NewClient(); defer cleanup()` 的使用模式。
func NewClient() (*redis.Client, func(), error) {
	m, err := New()
	if err != nil {
		return nil, nil, err
	}
	return m.Client(), m.Close, nil
}
