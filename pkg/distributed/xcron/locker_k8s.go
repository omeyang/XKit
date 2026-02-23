package xcron

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// 包级预编译的正则表达式，避免每次调用时重复编译
var (
	k8sNameReplaceRegex  = regexp.MustCompile(`[^a-z0-9-]`)
	k8sNameCollapseRegex = regexp.MustCompile(`-+`)
)

// K8sLocker 基于 K8S Lease 的分布式锁。
//
// 使用 K8S coordination.k8s.io/v1 Lease 资源实现分布式锁，
// 类似于 Leader Election 机制。
//
// 适用于：
//   - 多副本部署
//   - 离线环境（无 Redis）
//   - K8S 原生环境
//
// 前置条件：
//   - Pod 的 ServiceAccount 需要 Lease 资源的 get/create/update 权限
//   - K8S API Server 可用
//
// 用法：
//
//	locker, err := xcron.NewK8sLocker(xcron.K8sLockerOptions{
//	    Namespace: "my-namespace",
//	    Identity:  os.Getenv("POD_NAME"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	scheduler := xcron.New(xcron.WithLocker(locker))
type K8sLocker struct {
	client    kubernetes.Interface
	namespace string
	identity  string        // 实例标识（用于日志和调试）
	prefix    string        // Lease 名称前缀
	clockSkew time.Duration // 时钟偏移容忍度
}

// k8sLockHandle 表示一次成功的 K8s Lease 锁获取
type k8sLockHandle struct {
	locker    *K8sLocker
	key       string // 原始 key
	leaseName string // K8s Lease 资源名称
	token     string // 唯一 token（每次获取独立生成）
}

// K8sLockerOptions K8sLocker 配置选项
type K8sLockerOptions struct {
	// Namespace K8S 命名空间。
	// 默认从环境变量 POD_NAMESPACE 读取，或 "default"。
	Namespace string

	// Identity 当前实例标识。
	// 默认从环境变量 POD_NAME 读取，或 hostname:pid。
	// 用于日志记录和调试，实际锁值使用每次获取生成的唯一 token。
	Identity string

	// Prefix Lease 资源名称前缀。
	// 默认 "xcron-"。
	Prefix string

	// Client 自定义 K8S 客户端。
	// 默认使用 InClusterConfig 创建。
	Client kubernetes.Interface

	// ClockSkew 时钟偏移容忍度。
	// 用于判断 Lease 是否过期时的安全边界，防止节点间时钟不同步导致的问题。
	// 默认 2 秒。设置为负值表示禁用容忍度（不推荐）。
	// 注意：0 值会使用默认值 2 秒，若需禁用请设置为负值如 -1。
	ClockSkew time.Duration
}

// DefaultClockSkew 是默认的时钟偏移容忍度。
// K8s 官方 leader-election 库使用的默认值也是 2 秒。
const DefaultClockSkew = 2 * time.Second

// NewK8sLocker 创建基于 K8S Lease 的分布式锁。
//
// 如果不提供 Client，将使用 InClusterConfig 自动创建，
// 适用于在 K8S Pod 内运行的场景。
func NewK8sLocker(opts K8sLockerOptions) (*K8sLocker, error) {
	// 设置默认值
	if opts.Namespace == "" {
		opts.Namespace = getEnvOrDefault("POD_NAMESPACE", "default")
	}
	if opts.Identity == "" {
		opts.Identity = getEnvOrDefault("POD_NAME", defaultIdentity())
	}
	if opts.Prefix == "" {
		opts.Prefix = "xcron-"
	}
	if opts.ClockSkew == 0 {
		opts.ClockSkew = DefaultClockSkew
	} else if opts.ClockSkew < 0 {
		opts.ClockSkew = 0 // 用户显式禁用容忍度
	}

	// 创建 K8S 客户端
	client := opts.Client
	if client == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("xcron: failed to get in-cluster config: %w", err)
		}
		client, err = kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("xcron: failed to create k8s client: %w", err)
		}
	}

	return &K8sLocker{
		client:    client,
		namespace: opts.Namespace,
		identity:  opts.Identity,
		prefix:    opts.Prefix,
		clockSkew: opts.ClockSkew,
	}, nil
}

// TryLock 尝试获取锁（非阻塞）。
//
// 每次调用生成唯一 token，确保不同获取之间不会互相干扰。
// 通过创建或更新 Lease 资源实现锁获取。
//
// ttl 必须为正值，否则返回 [ErrInvalidTTL]。
func (l *K8sLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	leaseName := l.leaseName(key)
	// 每次获取生成唯一 token，包含实例标识便于调试
	token := fmt.Sprintf("%s:%s", l.identity, uuid.New().String())
	now := metav1.NewMicroTime(time.Now())
	leaseDuration := int32(ttl.Seconds())

	// 尝试获取现有 Lease
	lease, err := l.client.CoordinationV1().Leases(l.namespace).Get(ctx, leaseName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// Lease 不存在，创建新的
		handle, err := l.createLease(ctx, key, leaseName, token, leaseDuration, now)
		return handle, err
	}
	if err != nil {
		return nil, fmt.Errorf("xcron: failed to get lease: %w", err)
	}

	// Lease 存在，检查是否可以获取
	if l.canAcquire(lease) {
		return l.acquireLease(ctx, key, leaseName, token, lease, leaseDuration, now)
	}

	return nil, nil // 被其他实例持有
}

// Identity 返回当前实例标识。
func (l *K8sLocker) Identity() string {
	return l.identity
}

// Namespace 返回使用的 K8S 命名空间。
func (l *K8sLocker) Namespace() string {
	return l.namespace
}

// Client 返回底层 K8S 客户端。
func (l *K8sLocker) Client() kubernetes.Interface {
	return l.client
}

// createLease 创建新的 Lease
func (l *K8sLocker) createLease(ctx context.Context, key, leaseName, token string, duration int32, now metav1.MicroTime) (LockHandle, error) {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: l.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "xcron",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &token,
			LeaseDurationSeconds: &duration,
			AcquireTime:          &now,
			RenewTime:            &now,
		},
	}

	_, err := l.client.CoordinationV1().Leases(l.namespace).Create(ctx, lease, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// 并发创建，另一个实例先创建了
			return nil, nil
		}
		return nil, fmt.Errorf("xcron: failed to create lease: %w", err)
	}

	return &k8sLockHandle{
		locker:    l,
		key:       key,
		leaseName: leaseName,
		token:     token,
	}, nil
}

// acquireLease 获取已存在的 Lease
func (l *K8sLocker) acquireLease(ctx context.Context, key, leaseName, token string, lease *coordinationv1.Lease, duration int32, now metav1.MicroTime) (LockHandle, error) {
	lease.Spec.HolderIdentity = &token
	lease.Spec.LeaseDurationSeconds = &duration
	lease.Spec.AcquireTime = &now
	lease.Spec.RenewTime = &now

	_, err := l.client.CoordinationV1().Leases(l.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsConflict(err) {
			// 版本冲突，另一个实例先获取了
			return nil, nil
		}
		return nil, fmt.Errorf("xcron: failed to acquire lease: %w", err)
	}

	return &k8sLockHandle{
		locker:    l,
		key:       key,
		leaseName: leaseName,
		token:     token,
	}, nil
}

// canAcquire 判断是否可以获取 Lease
func (l *K8sLocker) canAcquire(lease *coordinationv1.Lease) bool {
	// 无持有者
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		return true
	}

	// 注意：即使是自己的 token 也不允许重入，防止重叠调度导致并发执行。
	// 场景：任务 A 执行中，任务 B 重叠触发，若允许 B 获取锁，
	// B 完成后 Unlock 会释放锁，此时 A 仍在执行但锁已被释放，
	// 其他 Pod 可获取锁导致并发。

	// 检查是否过期
	return l.isLeaseExpired(lease)
}

// isLeaseExpired 判断 Lease 是否已过期
// 使用时钟偏移容忍度来防止节点间时钟不同步导致的误判
func (l *K8sLocker) isLeaseExpired(lease *coordinationv1.Lease) bool {
	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}

	// 计算过期时间：renewTime + duration + clockSkew
	// 加上 clockSkew 是为了给持有者更多时间续期，防止因时钟偏移导致锁被抢占
	leaseDuration := time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	expireTime := lease.Spec.RenewTime.Add(leaseDuration + l.clockSkew)
	return time.Now().After(expireTime)
}

// Unlock 释放锁。
//
// 清除 Lease 的 holderIdentity，允许其他实例获取。
func (h *k8sLockHandle) Unlock(ctx context.Context) error {
	lease, err := h.locker.client.CoordinationV1().Leases(h.locker.namespace).Get(ctx, h.leaseName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // Lease 已被删除
		}
		return fmt.Errorf("xcron: failed to get lease for unlock: %w", err)
	}

	// 验证是否是自己持有（使用唯一 token 验证）
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != h.token {
		return ErrLockNotHeld
	}

	// 清除持有者
	lease.Spec.HolderIdentity = nil
	lease.Spec.AcquireTime = nil
	lease.Spec.RenewTime = nil

	_, err = h.locker.client.CoordinationV1().Leases(h.locker.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	if err != nil {
		// 如果是版本冲突，可能已被其他实例接管
		if errors.IsConflict(err) {
			return ErrLockNotHeld
		}
		return fmt.Errorf("xcron: failed to release lease: %w", err)
	}

	return nil
}

// Renew 续期锁。
//
// 更新 Lease 的 renewTime，延长锁的有效期。
// ttl 必须为正值，否则返回 [ErrInvalidTTL]。
func (h *k8sLockHandle) Renew(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	lease, err := h.locker.client.CoordinationV1().Leases(h.locker.namespace).Get(ctx, h.leaseName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("xcron: failed to get lease for renew: %w", err)
	}

	// 验证是否是自己持有（使用唯一 token 验证）
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != h.token {
		return ErrLockNotHeld
	}

	// 更新续期时间和租期
	now := metav1.NewMicroTime(time.Now())
	leaseDuration := int32(ttl.Seconds())
	lease.Spec.RenewTime = &now
	lease.Spec.LeaseDurationSeconds = &leaseDuration

	_, err = h.locker.client.CoordinationV1().Leases(h.locker.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsConflict(err) {
			return ErrLockNotHeld
		}
		return fmt.Errorf("xcron: failed to renew lease: %w", err)
	}

	return nil
}

// Key 返回锁的 key。
func (h *k8sLockHandle) Key() string {
	return h.key
}

// leaseName 生成 Lease 资源名称。
// 设计决策: 以最终名称（prefix + sanitized key）整体做 63 字符约束，
// 而非仅约束 key 部分，确保 K8s metadata.name 始终合法。
func (l *K8sLocker) leaseName(key string) string {
	return l.prefix + sanitizeK8sName(key, len(l.prefix))
}

// sanitizeK8sName 将字符串转换为合法的 K8S 资源名（不含 prefix 部分）。
// K8S 资源名要求：小写字母、数字、'-'，最终名称（prefix + 本函数输出）不超过 63。
// prefixLen 为调用方已占用的前缀长度，本函数输出不超过 63 - prefixLen。
//
// 当清理步骤改变了名称（字符替换/折叠/修剪）或长度超限时，
// 添加基于原始名称的 hash 后缀确保唯一性。
// 防止不同原始名称（如 "my.job" 和 "my/job"）清理后碰撞为 "my-job"。
func sanitizeK8sName(name string, prefixLen int) string {
	if name == "" {
		return ""
	}

	original := name

	// 转小写
	lowered := strings.ToLower(name)
	// 替换非法字符为 '-'（使用预编译的正则）
	sanitized := k8sNameReplaceRegex.ReplaceAllString(lowered, "-")
	// 去除连续的 '-'（使用预编译的正则）
	sanitized = k8sNameCollapseRegex.ReplaceAllString(sanitized, "-")
	// 去除首尾的 '-'
	sanitized = strings.Trim(sanitized, "-")

	const k8sMaxLen = 63
	const hashLen = 8
	const separator = "-"

	// 本函数输出的最大长度（为 prefix 预留空间）
	maxLen := k8sMaxLen - prefixLen
	if maxLen < 1 {
		maxLen = 1
	}

	// 当清理步骤改变了名称或长度超限时，添加 hash 后缀确保唯一性
	if sanitized != lowered || len(sanitized) > maxLen {
		hash := sha256.Sum256([]byte(original))
		hashSuffix := hex.EncodeToString(hash[:])[:hashLen]

		maxPrefixLen := max(maxLen-len(separator)-hashLen, 1)

		if len(sanitized) > maxPrefixLen {
			sanitized = sanitized[:maxPrefixLen]
			sanitized = strings.TrimRight(sanitized, "-")
		}

		return sanitized + separator + hashSuffix
	}

	return sanitized
}

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// 确保 K8sLocker 实现了 Locker 接口
var _ Locker = (*K8sLocker)(nil)

// 确保 k8sLockHandle 实现了 LockHandle 接口
var _ LockHandle = (*k8sLockHandle)(nil)
