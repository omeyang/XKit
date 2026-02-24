package xsemaphore

import (
	"context"
	"time"
)

// =============================================================================
// Permit - 许可句柄接口
// =============================================================================

// Permit 表示一次成功的许可获取。
//
// 每次 TryAcquire/Acquire 成功都会返回一个新的 Permit，内部封装了唯一标识。
// 通过 Permit 进行 Release 和 Extend 操作，确保不同获取之间不会互相干扰。
//
// # 设计目的
//
// Permit 模式解决了传统信号量接口的几个问题：
//   - 避免同一进程内多个 goroutine 使用同一信号量实例导致的状态混乱
//   - 每次获取许可时生成唯一标识，只有持有该标识的 permit 才能操作
//   - 更清晰的所有权语义：持有 permit 即持有许可
//
// # 使用模式
//
//	permit, err := sem.TryAcquire(ctx, "resource", xsemaphore.WithCapacity(100))
//	if err != nil {
//	    return err // 信号量服务异常
//	}
//	if permit == nil {
//	    return nil // 容量已满，稍后重试
//	}
//	defer permit.Release(ctx)
//
//	// 执行任务...
type Permit interface {
	// Release 释放许可。
	//
	// 只释放本次获取的许可，不会影响其他 goroutine 或实例持有的许可。
	// 返回 [ErrPermitNotHeld] 表示许可已过期或被其他获取覆盖。
	Release(ctx context.Context) error

	// Extend 续期许可。
	//
	// 延长许可的 TTL，用于长时间运行的任务。
	// 续期时间使用创建许可时配置的 TTL。
	//
	// 返回 [ErrPermitNotHeld] 表示许可已过期或被其他获取覆盖。
	// 其他错误表示续期操作失败（如网络错误），可以重试。
	Extend(ctx context.Context) error

	// StartAutoExtend 启动自动续租。
	//
	// 以指定间隔周期性调用 Extend，适用于运行时间不确定的长任务。
	// 返回 stop 函数，调用后停止自动续租。
	//
	// 建议 interval 小于 TTL 的一半，确保续租在过期前完成。
	// 例如：TTL=5分钟时，interval=1分钟是合理的。
	//
	// 使用示例：
	//
	//	stop := permit.StartAutoExtend(time.Minute)
	//	defer stop()
	//	// 执行长时间任务...
	StartAutoExtend(interval time.Duration) (stop func())

	// ID 返回许可的唯一标识。
	//
	// 用于日志记录和调试。
	ID() string

	// Resource 返回资源名称。
	Resource() string

	// TenantID 返回租户 ID。
	//
	// 如果未设置租户配额，返回空字符串。
	TenantID() string

	// ExpiresAt 返回许可的过期时间。
	ExpiresAt() time.Time

	// Metadata 返回许可的元数据。
	//
	// 元数据在获取许可时通过 WithMetadata 设置，用于携带业务上下文信息。
	// 返回的是副本，修改不会影响原始数据。
	// 如果未设置元数据，返回 nil。
	Metadata() map[string]string
}

// =============================================================================
// Semaphore - 信号量工厂接口
// =============================================================================

// Semaphore 定义信号量工厂接口。
// 工厂管理底层连接，并提供许可操作。
type Semaphore interface {
	// TryAcquire 非阻塞式获取许可。
	//
	// 每次调用生成唯一标识，确保不同获取之间不会互相干扰。
	// 成功时返回 Permit，容量已满时返回 (nil, nil)。
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制
	//   - resource: 资源标识，建议使用业务语义明确的名称
	//   - opts: 获取配置选项（如 WithCapacity 设置容量）
	//
	// 返回：
	//   - permit: 成功时返回 Permit，容量已满返回 nil
	//   - err: 服务异常（如 Redis 不可用）
	//
	// 注意：permit=nil 且 err=nil 表示容量已满，这是正常情况。
	TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error)

	// Acquire 阻塞式获取许可。
	//
	// 会根据配置的重试策略进行重试，直到获取到许可或 context 取消/超时。
	// 成功时返回 Permit。
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制
	//   - resource: 资源标识
	//   - opts: 获取配置选项
	//
	// 错误：
	//   - context.Canceled: context 被取消
	//   - context.DeadlineExceeded: context 超时
	//   - ErrAcquireFailed: 重试耗尽仍未获取到许可
	Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error)

	// Query 查询资源的当前状态。
	//
	// 返回全局和租户级别的许可使用情况。
	// 不消耗许可，用于监控和展示。
	//
	// 参数：
	//   - ctx: 上下文
	//   - resource: 资源标识
	//   - opts: 查询配置选项（如 WithTenantID 查询特定租户）
	//
	// 返回：
	//   - info: 资源使用信息
	//   - err: 查询失败时的错误
	Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error)

	// Close 关闭信号量，释放底层资源。
	// 关闭后不应再创建新的许可。已获取的许可仍可正常 Release 和 Extend。
	//
	// 设计决策: ctx 参数当前未使用，仅为接口统一而保留。
	// 关闭操作是幂等的，重复调用直接返回 nil。
	Close(ctx context.Context) error

	// Health 健康检查。
	// 检查底层连接是否正常。
	Health(ctx context.Context) error
}

// =============================================================================
// 数据结构
// =============================================================================

// ResourceInfo 资源使用信息
type ResourceInfo struct {
	// Resource 资源名称
	Resource string

	// GlobalCapacity 全局容量上限
	GlobalCapacity int

	// GlobalUsed 全局已使用许可数
	GlobalUsed int

	// GlobalAvailable 全局可用许可数
	GlobalAvailable int

	// TenantID 租户 ID（如果查询了租户信息）
	TenantID string

	// TenantQuota 租户配额上限
	TenantQuota int

	// TenantUsed 租户已使用许可数
	TenantUsed int

	// TenantAvailable 租户可用许可数
	TenantAvailable int
}
