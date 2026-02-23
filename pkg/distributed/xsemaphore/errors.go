package xsemaphore

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// 预定义错误
// =============================================================================

// 预定义错误，使用 errors.Is 进行比较
var (
	// ErrCapacityFull 全局容量已满。
	// TryAcquire 容量已满时返回 (nil, nil)，此错误用于日志和指标。
	ErrCapacityFull = errors.New("xsemaphore: global capacity is full")

	// ErrTenantQuotaExceeded 租户配额已满。
	// TryAcquire 租户配额已满时返回 (nil, nil)，此错误用于日志和指标。
	ErrTenantQuotaExceeded = errors.New("xsemaphore: tenant quota exceeded")

	// ErrPermitNotHeld 许可未被持有。
	// 尝试 Release 或 Extend 未持有的许可时返回此错误。
	ErrPermitNotHeld = errors.New("xsemaphore: permit not held")

	// ErrSemaphoreClosed 信号量已关闭。
	// 在已关闭的信号量上操作时返回此错误。
	ErrSemaphoreClosed = errors.New("xsemaphore: semaphore is closed")

	// ErrRedisUnavailable Redis 不可用。
	// Redis 连接失败时返回此错误。
	ErrRedisUnavailable = errors.New("xsemaphore: redis unavailable")

	// ErrAcquireFailed 获取许可失败。
	// 重试耗尽或其他获取失败的情况返回此错误。
	ErrAcquireFailed = errors.New("xsemaphore: failed to acquire permit")

	// ErrNilClient 客户端为空。
	// 传入 nil Redis 客户端时返回此错误。
	ErrNilClient = errors.New("xsemaphore: client is nil")

	// ErrInvalidCapacity 无效的容量配置。
	// 容量配置不合法时返回此错误。
	ErrInvalidCapacity = errors.New("xsemaphore: invalid capacity")

	// ErrInvalidTTL 无效的 TTL 配置。
	// TTL 配置不合法时返回此错误。
	ErrInvalidTTL = errors.New("xsemaphore: invalid ttl")

	// ErrInvalidTenantQuota 无效的租户配额配置。
	// 租户配额配置不合法时返回此错误。
	ErrInvalidTenantQuota = errors.New("xsemaphore: invalid tenant quota")

	// ErrInvalidResource 无效的资源名称。
	// 资源名称为空时返回此错误。
	ErrInvalidResource = errors.New("xsemaphore: invalid resource name")

	// ErrInvalidTenantID 无效的租户 ID。
	// 租户 ID 包含特殊字符（{, }, :）或空白字符时返回此错误。
	ErrInvalidTenantID = errors.New("xsemaphore: invalid tenant ID")

	// ErrInvalidKeyPrefix 无效的 key 前缀。
	// key 前缀包含 {} 时返回此错误（会破坏 Redis Cluster hash tag）。
	ErrInvalidKeyPrefix = errors.New("xsemaphore: invalid key prefix")

	// ErrUnknownScriptStatus Lua 脚本返回未知状态码。
	// 当 Redis Lua 脚本返回预期范围外的状态码时返回此错误。
	ErrUnknownScriptStatus = errors.New("xsemaphore: unknown script status code")

	// ErrIDGenerationFailed 许可 ID 生成失败。
	// 当 xid 生成器因时钟严重回拨等原因无法生成 ID 时返回此错误。
	ErrIDGenerationFailed = errors.New("xsemaphore: failed to generate permit ID")

	// ErrInvalidMaxRetries 无效的最大重试次数配置。
	// 重试次数必须为正整数时返回此错误。
	ErrInvalidMaxRetries = errors.New("xsemaphore: invalid max retries")

	// ErrInvalidRetryDelay 无效的重试间隔配置。
	// 重试间隔必须为正数时返回此错误。
	ErrInvalidRetryDelay = errors.New("xsemaphore: invalid retry delay")

	// ErrNilContext context 参数为空。
	// 所有公开方法都要求传入非 nil 的 context.Context。
	// 设计决策: Close 方法例外，不校验 ctx（Close 不使用 context，参数仅为接口统一而保留）。
	ErrNilContext = errors.New("xsemaphore: context must not be nil")

	// ErrInvalidPodCount 无效的 Pod 数量配置。
	// Pod 数量必须为正整数。
	ErrInvalidPodCount = errors.New("xsemaphore: invalid pod count")

	// ErrInvalidFallbackStrategy 无效的降级策略。
	// 降级策略必须为 FallbackLocal、FallbackOpen 或 FallbackClose。
	ErrInvalidFallbackStrategy = errors.New("xsemaphore: invalid fallback strategy")

	// errUnexpectedScriptResult Lua 脚本返回结果不符合预期（内部使用）
	errUnexpectedScriptResult = errors.New("xsemaphore: unexpected script result")
)

// =============================================================================
// 获取失败原因
// =============================================================================

// AcquireFailReason 获取失败的原因
type AcquireFailReason int

const (
	// ReasonUnknown 未知原因
	ReasonUnknown AcquireFailReason = iota

	// ReasonCapacityFull 全局容量已满
	ReasonCapacityFull

	// ReasonTenantQuotaExceeded 租户配额已满
	ReasonTenantQuotaExceeded
)

// String 返回失败原因的字符串表示
func (r AcquireFailReason) String() string {
	switch r {
	case ReasonCapacityFull:
		return "capacity_full"
	case ReasonTenantQuotaExceeded:
		return "tenant_quota_exceeded"
	default:
		return "unknown"
	}
}

// Error 返回对应的错误
func (r AcquireFailReason) Error() error {
	switch r {
	case ReasonCapacityFull:
		return ErrCapacityFull
	case ReasonTenantQuotaExceeded:
		return ErrTenantQuotaExceeded
	default:
		return nil
	}
}

// =============================================================================
// 错误检查函数
// =============================================================================

// redisRelatedErrors 包含所有需要检查的 Redis 相关错误
var redisRelatedErrors = []error{
	ErrRedisUnavailable,
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
	syscall.EPIPE,
	syscall.ETIMEDOUT,
	io.EOF,
	io.ErrUnexpectedEOF,
}

// IsRedisError 检查是否是 Redis 相关错误
//
// 使用类型断言和错误链检查，而不是字符串匹配。
// 支持检测 Redis Cluster 相关错误（CLUSTERDOWN, MOVED, ASK, READONLY, CROSSSLOT）。
// 支持检测 Redis 代理能力限制错误（如 Twemproxy 不支持 EVAL）。
// 注意：context.Canceled 和 context.DeadlineExceeded 不被视为 Redis 错误，
// 因为这些是客户端超时，不应触发降级。
func IsRedisError(err error) bool {
	if err == nil {
		return false
	}

	// 排除 context 错误，这些是客户端超时，不应触发降级
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// 检查已知的错误类型
	for _, target := range redisRelatedErrors {
		if errors.Is(err, target) {
			return true
		}
	}

	// 检查 Redis Cluster 相关错误
	// 这些错误表明 Redis 集群处于不健康状态，应触发降级
	if isRedisClusterError(err) {
		return true
	}

	// 检查 Redis 协议错误（代理能力限制等）
	// 例如 Twemproxy 返回 "ERR unknown command 'eval'"
	if isRedisProtocolError(err) {
		return true
	}

	// 检查网络相关错误
	return isNetworkError(err)
}

// isRedisClusterError 检查是否是 Redis Cluster 相关错误
// 包括 CLUSTERDOWN、MOVED、ASK、READONLY、CROSSSLOT 等
//
// 设计决策：
//   - MOVED/ASK: go-redis 会自动处理重定向，传到应用层说明重定向失败，应触发降级
//   - TRYAGAIN: 不触发降级，这是临时状态，应由重试机制处理
//   - CLUSTERDOWN/MASTERDOWN/LOADING: 集群故障，触发降级
func isRedisClusterError(err error) bool {
	// CLUSTERDOWN: 集群处于 fail 状态
	if redis.IsClusterDownError(err) {
		return true
	}

	// MOVED: 键所在的槽已迁移到其他节点
	// 注意：go-redis 通常会自动处理 MOVED，但脚本执行可能触发
	if _, ok := redis.IsMovedError(err); ok {
		return true
	}

	// ASK: 键正在迁移中
	if _, ok := redis.IsAskError(err); ok {
		return true
	}

	// READONLY: 节点处于只读状态（主节点故障转移期间）
	if redis.IsReadOnlyError(err) {
		return true
	}

	// CROSSSLOT: 键不在同一个槽（多键操作失败）
	if errors.Is(err, redis.ErrCrossSlot) {
		return true
	}

	// 注意：TRYAGAIN 不触发降级
	// TRYAGAIN 是临时状态（通常在槽迁移期间），应由重试机制处理而非触发降级
	// 参见 isRetryableRedisError 函数

	// MASTERDOWN: 主节点不可用
	if redis.IsMasterDownError(err) {
		return true
	}

	// LOADING: Redis 正在加载数据
	if redis.IsLoadingError(err) {
		return true
	}

	return false
}

// isRedisProtocolError 检查是否是 Redis 协议错误（代理能力限制等）
//
// 当代理（如 Twemproxy、Codis）不支持某些命令时，会返回类似以下错误：
//   - "ERR unknown command 'eval'"
//   - "ERR unknown command 'evalsha'"
//   - "ERR This instance has cluster support disabled"
//
// go-redis 将这些错误作为 redis.Error 类型返回（实际是字符串类型）。
// 这些错误表明 Redis 代理能力受限，应触发降级策略。
func isRedisProtocolError(err error) bool {
	if err == nil {
		return false
	}

	// go-redis 的 redis.Error 是字符串类型别名
	// 检查错误消息中的关键词来识别代理能力限制
	errStr := err.Error()

	// 检测不支持的命令错误
	// 例如：Twemproxy 不支持 EVAL/EVALSHA
	if strings.Contains(errStr, "unknown command") {
		return true
	}

	// NOSCRIPT 通常由 go-redis 自动处理（回退到 EVAL）
	// 如果传到应用层，说明 EVAL 也失败了，应触发降级
	if strings.Contains(errStr, "NOSCRIPT") {
		return true
	}

	// 集群支持被禁用
	if strings.Contains(errStr, "cluster support disabled") {
		return true
	}

	return false
}

// isNetworkError 检查是否是网络相关错误
func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// IsCapacityFull 检查是否是容量已满错误
func IsCapacityFull(err error) bool {
	return errors.Is(err, ErrCapacityFull)
}

// IsTenantQuotaExceeded 检查是否是租户配额已满错误
func IsTenantQuotaExceeded(err error) bool {
	return errors.Is(err, ErrTenantQuotaExceeded)
}

// IsPermitNotHeld 检查是否是许可未持有错误
func IsPermitNotHeld(err error) bool {
	return errors.Is(err, ErrPermitNotHeld)
}

// isRetryableRedisError 检查是否是应该重试的 Redis 错误
// 仅包括临时性错误，不包括需要降级处理的错误
//
// 设计说明：
//   - TRYAGAIN: 槽迁移期间的临时状态，应重试而非立即失败
//   - 其他 Redis 错误（CLUSTERDOWN、网络错误等）属于致命错误，应触发降级
func isRetryableRedisError(err error) bool {
	if err == nil {
		return false
	}
	// TRYAGAIN: 槽迁移期间的临时状态
	return redis.IsTryAgainError(err)
}

// IsRetryable 检查错误是否可重试
//
// 容量已满和租户配额已满不可重试（需要等待释放）。
// Redis 错误可重试。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// 容量/配额错误不可重试
	if IsCapacityFull(err) || IsTenantQuotaExceeded(err) {
		return false
	}
	// Redis 错误可重试
	return IsRedisError(err)
}

// =============================================================================
// 错误分类（用于低基数指标）
// =============================================================================

// 错误分类常量
const (
	// ErrClassRedisUnavailable Redis 不可用
	ErrClassRedisUnavailable = "redis_unavailable"
	// ErrClassPermitNotHeld 许可未持有
	ErrClassPermitNotHeld = "permit_not_held"
	// ErrClassTimeout 超时
	ErrClassTimeout = "timeout"
	// ErrClassCanceled 取消
	ErrClassCanceled = "canceled"
	// ErrClassInternal 内部错误
	ErrClassInternal = "internal_error"
)

// ClassifyError 将错误分类为低基数字符串
// 用于指标属性，避免高基数标签导致的内存问题
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}
	if IsRedisError(err) {
		return ErrClassRedisUnavailable
	}
	if IsPermitNotHeld(err) {
		return ErrClassPermitNotHeld
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrClassTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrClassCanceled
	}
	return ErrClassInternal
}
