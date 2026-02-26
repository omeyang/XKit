package xetcd

import (
	"context"
	"fmt"
	"math"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Get 获取键值。
//
// 参数：
//   - ctx: 上下文
//   - key: 键名
//
// 返回：
//   - []byte: 键值
//   - error: 获取失败时返回错误，键不存在返回 ErrKeyNotFound
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	value, _, err := c.GetWithRevision(ctx, key)
	return value, err
}

// GetWithRevision 获取键值和版本号。
//
// 参数：
//   - ctx: 上下文
//   - key: 键名
//
// 返回：
//   - []byte: 键值
//   - int64: 版本号（ModRevision）
//   - error: 获取失败时返回错误
func (c *Client) GetWithRevision(ctx context.Context, key string) ([]byte, int64, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return nil, 0, err
	}
	if key == "" {
		return nil, 0, ErrEmptyKey
	}

	resp, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, 0, fmt.Errorf("xetcd: get %q: %w", key, err)
	}

	if len(resp.Kvs) == 0 {
		return nil, 0, ErrKeyNotFound
	}

	kv := resp.Kvs[0]
	return kv.Value, kv.ModRevision, nil
}

// Put 写入键值。
func (c *Client) Put(ctx context.Context, key string, value []byte) error {
	if err := c.checkPreconditions(ctx); err != nil {
		return err
	}
	if key == "" {
		return ErrEmptyKey
	}

	_, err := c.client.Put(ctx, key, string(value))
	if err != nil {
		return fmt.Errorf("xetcd: put %q: %w", key, err)
	}
	return nil
}

// PutWithTTL 写入带 TTL 的键值。
// 键值会在 TTL 到期后自动删除。
// 设计决策: TTL 使用向上取整（ceil）转换为秒，确保键不会比调用方预期更早过期。
// 例如 1.1s 会被转换为 2s，而非向下取整为 1s。
func (c *Client) PutWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.checkPreconditions(ctx); err != nil {
		return err
	}
	if key == "" {
		return ErrEmptyKey
	}
	// 设计决策: ttl <= 0 时降级为普通 Put（永不过期），而非返回错误。
	// 这简化了调用方的动态 TTL 计算场景（计算结果可能为 0），
	// 同时与 Go 标准库零值行为一致（如 time.Duration 零值 = 无等待）。
	if ttl <= 0 {
		return c.Put(ctx, key, value)
	}

	// 创建租约
	// 设计决策: 使用向上取整确保键存活时间不短于调用方要求。
	ttlSeconds := int64(math.Ceil(ttl.Seconds()))
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	lease, err := c.client.Grant(ctx, ttlSeconds)
	if err != nil {
		return fmt.Errorf("xetcd: grant lease: %w", err)
	}

	// 写入带租约的键值
	_, err = c.client.Put(ctx, key, string(value), clientv3.WithLease(lease.ID))
	if err != nil {
		// Put 失败时尝试撤销租约，避免租约泄漏
		// 使用 Background context 确保即使原 ctx 已取消也能执行撤销
		c.tryRevokeLease(lease.ID)
		return fmt.Errorf("xetcd: put %q with ttl: %w", key, err)
	}
	return nil
}

// Delete 删除键值。键不存在时不返回错误。
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.checkPreconditions(ctx); err != nil {
		return err
	}
	if key == "" {
		return ErrEmptyKey
	}

	_, err := c.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("xetcd: delete %q: %w", key, err)
	}
	return nil
}

// DeleteWithPrefix 删除指定前缀的所有键，返回删除的键数量。
func (c *Client) DeleteWithPrefix(ctx context.Context, prefix string) (int64, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return 0, err
	}
	if prefix == "" {
		return 0, ErrEmptyKey
	}

	resp, err := c.client.Delete(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, fmt.Errorf("xetcd: delete prefix %q: %w", prefix, err)
	}
	return resp.Deleted, nil
}

// List 列出指定前缀的所有键值，返回键值对映射。
// 注意：此方法一次性加载所有匹配的键值到内存中，
// 不适用于前缀下有大量 key 的场景（如数万个服务实例）。
// 大量 key 场景请使用 RawClient() 配合 clientv3.WithLimit/clientv3.WithRange 自行分页。
func (c *Client) List(ctx context.Context, prefix string) (map[string][]byte, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return nil, err
	}
	if prefix == "" {
		return nil, ErrEmptyKey
	}

	resp, err := c.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("xetcd: list %q: %w", prefix, err)
	}

	result := make(map[string][]byte, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		result[string(kv.Key)] = kv.Value
	}
	return result, nil
}

// ListKeys 仅列出键名，返回键名列表。
func (c *Client) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return nil, err
	}
	if prefix == "" {
		return nil, ErrEmptyKey
	}

	resp, err := c.client.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return nil, fmt.Errorf("xetcd: list keys %q: %w", prefix, err)
	}

	keys := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		keys = append(keys, string(kv.Key))
	}
	return keys, nil
}

// Exists 检查键是否存在。
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrEmptyKey
	}

	resp, err := c.client.Get(ctx, key, clientv3.WithCountOnly())
	if err != nil {
		return false, fmt.Errorf("xetcd: exists %q: %w", key, err)
	}
	return resp.Count > 0, nil
}

// Count 统计指定前缀的键数量。
func (c *Client) Count(ctx context.Context, prefix string) (int64, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return 0, err
	}
	if prefix == "" {
		return 0, ErrEmptyKey
	}

	resp, err := c.client.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithCountOnly())
	if err != nil {
		return 0, fmt.Errorf("xetcd: count %q: %w", prefix, err)
	}
	return resp.Count, nil
}

// revokeLeaseTimeout 租约撤销的超时时间。
// 设计决策: 使用 3 秒超时而非无限等待，因为租约最终会自动过期，
// 撤销仅是 best-effort 清理，不应阻塞调用方。
const revokeLeaseTimeout = 3 * time.Second

// tryRevokeLease 尝试撤销租约，用于清理场景。
// 设计决策: 撤销失败时静默处理（不返回错误、不记录日志），原因：
//   - 租约有 TTL，最终会自动过期，撤销仅是 best-effort 提前清理
//   - 此处添加日志需要引入 logger 依赖，与 xetcd 的简化封装定位不符
//   - 调用方已收到 Put 失败的错误，能感知到异常情况
//
// 使用独立的带超时 context，确保即使原 context 已取消也能执行，
// 同时避免 etcd 不可达时无限阻塞。
func (c *Client) tryRevokeLease(leaseID clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), revokeLeaseTimeout)
	defer cancel()
	_, err := c.client.Revoke(ctx, leaseID)
	if err != nil {
		// 租约撤销失败不影响主流程，租约会自动过期
		// 这里显式处理错误而非忽略，满足 errcheck 要求
		return
	}
}
