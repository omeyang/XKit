// Package xsemaphore 提供分布式信号量实现，用于限制跨多实例的资源并发访问。
//
// # 设计理念
//
// xsemaphore 采用与 xdlock 相同的 Factory + Handle 模式：
//   - Semaphore 工厂接口管理连接和配置
//   - Permit 句柄表示获取到的许可，提供 Release/Extend 操作
//   - TryAcquire 返回 (nil, nil) 表示容量已满
//
// 与分布式锁的区别：
//   - 锁是互斥的（同时只有一个持有者）
//   - 信号量支持多个并发许可（最多 N 个持有者）
//
// # 使用场景
//
//   - 限制多 K8s Pod 部署的资源并发访问总量（如 ClickHouse 推理任务最多 100 个）
//   - 支持租户级别的并发配额限制（如每租户最多 5 个并发任务）
//   - 长时间任务的 TTL 自动释放和续租机制
//
// # 核心概念
//
//   - Semaphore: 信号量工厂，管理连接和创建许可
//   - Permit: 单个许可句柄，提供 Release/Extend/StartAutoExtend 操作
//   - AcquireOption: 获取许可的配置选项
//   - Option: 工厂级配置选项
//
// # 快速开始
//
// 基本用法：
//
//	// 创建信号量
//	sem, err := xsemaphore.New(rdb,
//	    xsemaphore.WithFallback(xsemaphore.FallbackLocal),
//	    xsemaphore.WithPodCount(3),
//	)
//	if err != nil {
//	    return err
//	}
//	defer sem.Close(context.Background())
//
//	// 获取许可
//	permit, err := sem.TryAcquire(ctx, "clickhouse-inference",
//	    xsemaphore.WithCapacity(100),      // 全局最多 100 个并发
//	    xsemaphore.WithTenantQuota(5),     // 每租户最多 5 个
//	    xsemaphore.WithTTL(5*time.Minute),
//	)
//	if err != nil {
//	    return err
//	}
//	if permit == nil {
//	    return errors.New("system busy")
//	}
//	defer permit.Release(ctx)
//
//	// 执行任务...
//
// # 长任务自动续租
//
// 对于运行时间不确定的长任务，可以启动自动续租：
//
//	permit, _ := sem.Acquire(ctx, "long-task",
//	    xsemaphore.WithCapacity(10),
//	    xsemaphore.WithTTL(5*time.Minute),
//	)
//	defer permit.Release(ctx)
//
//	// 启动自动续租（每分钟续期一次）
//	stop := permit.StartAutoExtend(time.Minute)
//	defer stop()
//
//	// 执行长时间任务...
//
// # 租户配额限制
//
// 支持在全局容量基础上叠加租户级配额。租户配额仅在同时满足以下条件时启用：
//   - TenantID 非空（通过 WithTenantID 设置或从 context 自动提取）
//   - TenantQuota > 0（通过 WithTenantQuota 设置）
//
// 示例：
//
//	permit, err := sem.TryAcquire(ctx, "inference",
//	    xsemaphore.WithCapacity(100),    // 全局容量
//	    xsemaphore.WithTenantQuota(5),   // 租户配额
//	    xsemaphore.WithTenantID("t123"), // 租户 ID（可从 context 自动提取）
//	)
//
// 注意：如果只设置 TenantID 而不设置 TenantQuota（或 TenantQuota=0），
// 则不会创建租户级 Redis key，也不会进行租户配额检查。
//
// # 降级策略
//
// 当 Redis 不可用时，支持三种降级策略：
//
//   - FallbackLocal: 降级到本地信号量（本地容量 = 全局容量 / Pod 数量）
//   - FallbackOpen: 放行所有请求（fail-open），返回虚拟许可
//   - FallbackClose: 拒绝所有请求（fail-close），返回 ErrRedisUnavailable
//
// 各操作在不同策略下的行为：
//
//	| 操作       | FallbackLocal      | FallbackOpen       | FallbackClose      |
//	|------------|--------------------|--------------------|---------------------|
//	| TryAcquire | 使用本地信号量     | 返回虚拟许可       | 返回错误           |
//	| Acquire    | 使用本地信号量     | 返回虚拟许可       | 返回错误           |
//	| Query      | 查询本地状态       | 返回全部可用       | 返回错误           |
//
// 注意：context.Canceled 和 context.DeadlineExceeded 不会触发降级，
// 因为这些是客户端超时，不表示 Redis 不可用。
//
// # Permit ID 唯一性
//
// 每个 Permit 都有全局唯一的 ID（使用 Sonyflake 雪花算法生成），可用于：
//   - 日志追踪和调试
//   - 在 Release/Extend 操作中标识许可
//
// 相比 UUID，Sonyflake ID 具有以下优势：
//   - 更高效的生成速度（~50ns vs ~500ns）
//   - 更短的字符串长度（~13 字符 vs 36 字符）
//   - 具有时序性，便于排序和调试
//
// FallbackOpen 策略下的虚拟许可也有唯一 ID（格式为 "noop-{id}"）。
//
// # ID 生成错误处理
//
// xsemaphore 使用 xid.NewStringWithRetry() 生成许可 ID，该函数在时钟严重回拨时
// 会返回错误而非 panic。如果 ID 生成失败，Acquire/TryAcquire 会返回
// ErrIDGenerationFailed 错误。
//
// 这种设计确保时钟问题不会导致整个服务崩溃，而是返回可处理的错误。
// 建议在监控系统中设置对 ErrIDGenerationFailed 错误的告警。
//
// # 资源命名最佳实践
//
// 资源名称会作为指标标签，应避免使用动态生成的名称（如包含用户 ID），
// 否则会导致指标高基数问题。推荐使用静态的业务资源名：
//
//	// 推荐
//	sem.TryAcquire(ctx, "clickhouse-inference", ...)
//	sem.TryAcquire(ctx, "api-external-call", ...)
//
//	// 不推荐（会导致高基数）
//	sem.TryAcquire(ctx, "user-"+userID+"-task", ...)
//
// 如果确实需要动态资源名，可使用 WithDisableResourceLabel() 禁用指标中的
// resource 标签：
//
//	sem, _ := xsemaphore.New(rdb,
//	    xsemaphore.WithDisableResourceLabel(),
//	)
//
// # 数据结构
//
// Redis 存储使用 Sorted Set：
//
//	# 全局许可集合 - score=过期时间戳毫秒, member=permitID
//	{prefix}:{resource}:permits -> ZSET
//
//	# 租户许可集合（仅在 TenantID 非空且 TenantQuota > 0 时创建）
//	{prefix}:{resource}:t:{tenantID} -> ZSET
//
// # Redis Cluster 支持
//
// xsemaphore 使用 {resource} 作为 hash tag，确保同一资源的全局键和租户键
// 映射到同一 Redis Cluster slot，避免 CROSSSLOT 错误。键格式如下：
//
//	{prefix}{resource}:permits      -> 全局许可
//	{prefix}{resource}:t:{tenantID} -> 租户许可
//
// 注意：KEYS 数组是动态构建的，仅在需要租户配额时才传入租户键。
// 当 TenantID 为空或 TenantQuota=0 时，仅传递 1 个 key（全局键），
// 避免向 Redis Cluster 传递空字符串导致的潜在问题。
//
// 降级触发条件覆盖以下 Redis Cluster 错误：
//   - CLUSTERDOWN: 集群处于 fail 状态
//   - MOVED: 键所在的槽已迁移（重定向失败时）
//   - ASK: 键正在迁移中（重定向失败时）
//   - READONLY: 节点处于只读状态
//   - CROSSSLOT: 多键操作失败（理论上不应发生）
//   - MASTERDOWN: 主节点不可用
//   - LOADING: Redis 正在加载数据
//
// 注意：TRYAGAIN 不会触发降级，因为这是临时状态，应由重试机制处理。
//
// # FallbackLocal 容量策略
//
// 降级到本地信号量时，容量分摊策略为：
//
//	localCapacity = max(1, globalCapacity / podCount)
//
// 这意味着：
//   - 每个 Pod 至少保证 1 个本地许可（即使 globalCapacity < podCount）
//   - 总本地容量可能超过全局配置（podCount * localCapacity >= globalCapacity）
//   - 这是可用性优先的设计选择，确保降级时服务不完全中断
//
// 注意：Query 方法也使用相同的 max(1, ...) 策略计算本地容量，
// 确保 Query 返回的 GlobalCapacity 与 Acquire 实际使用的容量一致。
//
// 设计决策: FallbackLocal 恢复窗口 —— Redis 恢复后新请求立即走分布式路径，
// 此前在本地 fallback 中仍有效的 permit 不会回填到 Redis 账本。
// 恢复窗口内理论上可出现 local_active + redis_active > globalCapacity。
// 这是刻意的简单性选择：
//   - 超发上界为 localCapacity（= globalCapacity/podCount），且仅持续到本地 permit TTL 到期
//   - 引入"粘滞回切"或"permit 补登"机制会大幅增加复杂度，与 fallback 的"尽力而为"定位不符
//   - 业务方若需严格容量保证，应使用 FallbackClose 策略
//
// # 并发安全
//
// xsemaphore 的所有公开方法都是并发安全的：
//   - Semaphore 接口的所有方法可以被多个 goroutine 同时调用
//   - Permit 接口的方法（Release/Extend/StartAutoExtend）可以被多个 goroutine 同时调用
//   - 内部使用 sync.Map 和 sync.Mutex 保护共享状态
//
// localSemaphore 的 ensureLocalSemaphore 与 Close 之间通过 localMu + nil check + closed flag
// 三重保护互斥，确保 Close 后不会创建新的 localSemaphore，也不存在空指针风险。
//
// cleanupAllExpired 不删除空的 resourcePermits bucket。空 map 内存开销极小（~80 bytes），
// 而删除操作可能导致孤儿 bucket 竞态：另一个 goroutine 通过 getResourcePermits 持有旧引用，
// 写入的许可会进入已脱链的 bucket。这是正确性优先于微小内存节省的设计决策。
//
// # 延迟初始化
//
// 当使用 FallbackOpen 或 FallbackClose 策略时，localSemaphore 不会被创建，
// 避免不必要的资源开销（后台清理 goroutine、sync.Map 等）。
// localSemaphore 仅在 FallbackLocal 策略首次触发降级时才会被初始化。
//
// # 配置验证行为
//
// 选项函数（如 WithCapacity、WithTTL）直接设置用户传入的值，不做静默过滤。
// 工厂级选项（如 WithKeyPrefix、WithPodCount）由 New() 中的 validate() 校验。
// 操作级选项（如 WithCapacity、WithTTL）由 TryAcquire/Acquire/Query 中的 validate() 校验。
// 这种 fail-fast 模式确保无效配置能被及时发现，而非被静默忽略。
//
// # 时钟与过期处理
//
// 时间基准：xsemaphore 使用客户端时钟（Go 应用）而非 Redis 服务端时钟。
// 这是经过权衡的设计选择，与许多分布式锁库（如 redlock、go-redis/lock）一致：
//   - 优点：避免每次操作都调用 Redis TIME 命令增加延迟
//   - 缺点：跨 Pod 时钟漂移可能导致过期判定略有偏差
//   - 缓解：现代基础设施通常使用 NTP 保持时钟同步在毫秒级
//   - 建议：对于高精度要求的场景，确保 NTP 配置正确
//
// # K8s 集群内时钟同步
//
// xsemaphore 使用客户端时钟判断许可过期。在 K8s 环境中，Pod 之间的时钟
// 通常通过宿主机 NTP 同步，偏差在毫秒级别。
//
// ## 最佳实践
//
//   - 确保 K8s 节点启用 NTP 同步
//   - 使用较长的 TTL（>= 30 秒）留出时钟偏差余量
//   - 使用 StartAutoExtend 而非依赖精确过期
//   - 监控节点时钟偏差（如 node_timex_offset_seconds）
//
// ## 云厂商时钟同步配置
//
// 各云厂商 K8s 节点的 NTP 服务配置：
//
//   - GKE (Google): 默认使用 metadata.google.internal（169.254.169.254）
//     节点自动同步，无需额外配置
//
//   - EKS (AWS): 默认使用 Amazon Time Sync Service（169.254.169.123）
//     通过 chrony 自动配置，偏差通常在微秒级
//
//   - AKS (Azure): 默认使用 Azure 内置 NTP 服务
//     通过 systemd-timesyncd 或 chrony 同步
//
// ## 验证时钟同步状态
//
// 在 Pod 中检查时钟同步状态：
//
//	# 使用 chrony（常见于 EKS）
//	chronyc tracking
//
//	# 使用 systemd-timesyncd
//	timedatectl status
//
//	# 检查节点时钟偏差
//	kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes | jq '.items[].status.conditions'
//
// 时钟偏差超过 1 秒可能导致许可管理异常，建议在监控系统中设置告警。
//
// 过期边界：所有实现统一使用 <= now 语义（score <= now 视为已过期）：
//   - Redis Lua 写路径（acquire/extend）: ZREMRANGEBYSCORE 清理过期条目，extend 使用 <= 判断
//   - Redis Lua 读路径（query）: 使用 ZCOUNT 统计未过期条目，纯只读操作
//   - Local: 使用 !After(now) 等价于 <= now
//
// 资源名称验证：resource 参数会进行以下校验，不通过则返回 ErrInvalidResource：
//   - 不能为空字符串
//   - 不能包含特殊字符 `{`, `}`, `:` （这些字符会破坏 Redis key 结构和 hash tag）
//
// 示例：
//
//	// 合法的资源名
//	"inference-task"      // OK
//	"api.external.call"   // OK
//	"task_123"            // OK
//
//	// 非法的资源名
//	""                    // 空字符串
//	"task:123"            // 包含冒号
//	"user{123}"           // 包含花括号
//
// # Query 方法是只读操作
//
// Query 方法是纯只读操作，使用 ZCOUNT 统计未过期的许可数量，不会修改任何数据。
// 这意味着：
//   - 在读写分离的 Redis 部署中，Query 请求可安全路由到从节点
//   - Query 操作不影响写性能
//
// 过期许可的清理由 Acquire 和 Extend 的写路径负责（通过 ZREMRANGEBYSCORE），
// 因此 Query 返回的计数始终是准确的（排除了 score <= now 的过期条目）。
//
// # 设计说明：容量每次调用传入
//
// 容量（capacity）和租户配额（tenantQuota）在每次 Acquire 调用时传入，
// 而非在工厂创建时固定。这是有意的设计选择，与 xdlock 的模式一致：
//   - 提供灵活性，允许不同场景使用不同配置
//   - 业务层可通过封装确保一致性
//   - 避免工厂级配置与调用级配置的优先级混淆
//
// # Close() 行为
//
// Close() 方法会阻止新的 TryAcquire/Acquire/Query 操作，但不会影响已获取的许可：
//   - 已获取的 Permit 仍然可以调用 Release() 和 Extend()
//   - 自动续租（StartAutoExtend）会继续工作直到被停止或许可过期
//   - 这是设计选择，确保业务逻辑能够正常完成
//
// 推荐的关闭顺序：
//
//	// 1. 停止接受新请求（业务层）
//	// 2. 等待所有任务完成并释放许可
//	// 3. 关闭信号量
//	sem.Close(context.Background())
//
// # Redis 键 TTL 管理
//
// Redis 键的过期时间采用"只延长、不缩短"的策略：
//   - 每次 Acquire/Extend 时，仅当新 TTL 大于当前 TTL 时才更新键过期时间
//   - 这确保短 TTL 的许可不会影响同一资源下长 TTL 许可的生存周期
//   - 键的最终 TTL 始终等于该资源所有许可中最大的过期时间 + 余量
//
// # Redis 代理支持
//
// xsemaphore 使用 Lua 脚本执行原子操作，对 Redis 代理（如 Twemproxy、Codis）的支持：
//   - go-redis 库内部已处理 NOSCRIPT 错误：首次使用 EVALSHA，失败后自动回退到 EVAL
//   - 使用 {resource} 作为 hash tag，确保多键操作路由到同一节点
//   - 建议：在应用启动时调用 WarmupScripts() 预热脚本缓存
//
// 注意：部分代理可能不支持 Lua 脚本或 MULTI/EXEC，请查阅代理文档确认兼容性。
//
// 当代理不支持 EVAL/EVALSHA 命令时，会返回类似 "ERR unknown command 'eval'" 的错误。
// 这类错误会被 IsRedisError() 检测为 Redis 错误，从而触发降级策略。
//
// # Lua 脚本兼容性
//
// xsemaphore 依赖 Redis Lua 脚本实现原子操作，需要确保 Redis 部署支持 EVAL/EVALSHA 命令。
//
// 兼容性检测：
//   - 强烈建议在应用启动时调用 WarmupScripts() 预热并检测兼容性
//   - 如果 Redis 代理不支持 Lua 脚本，WarmupScripts() 会返回错误
//   - 这样可以在启动时就发现问题，而不是在运行时才遇到错误
//
// 示例：
//
//	if err := xsemaphore.WarmupScripts(ctx, client); err != nil {
//	    // Redis 代理可能不支持 Lua 脚本
//	    log.Fatalf("Redis Lua script not supported: %v", err)
//	}
//
// # WarmupScripts 最佳实践
//
// WarmupScripts 函数有三个主要用途：
//
// 1. 预加载脚本缓存：首次调用 EVALSHA 时，如果 Redis 没有缓存脚本，
// 会触发 NOSCRIPT 错误并自动回退到 EVAL（加载脚本）。WarmupScripts
// 在启动时预加载所有脚本到 Redis 缓存，避免首次请求的额外延迟。
//
// 2. 代理兼容性检测：部分 Redis 代理（如 Twemproxy、Codis）不支持 Lua 脚本。
// 在启动时调用 WarmupScripts 可以立即发现此问题，而非在运行时遇到错误。
//
// 3. Redis 连接验证：WarmupScripts 会尝试连接 Redis 并执行命令，
// 可以作为启动时的健康检查。
//
// 推荐的初始化模式：
//
//	func initSemaphore(rdb redis.UniversalClient) (*xsemaphore.Semaphore, error) {
//	    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	    defer cancel()
//
//	    // 预热脚本并检测兼容性
//	    if err := xsemaphore.WarmupScripts(ctx, rdb); err != nil {
//	        // 判断是网络错误还是脚本不支持
//	        if xsemaphore.IsRedisError(err) {
//	            return nil, fmt.Errorf("redis connection failed: %w", err)
//	        }
//	        return nil, fmt.Errorf("redis proxy may not support Lua scripts: %w", err)
//	    }
//
//	    // 创建信号量
//	    return xsemaphore.New(rdb,
//	        xsemaphore.WithFallback(xsemaphore.FallbackLocal),
//	        xsemaphore.WithPodCount(3),
//	    )
//	}
//
// 注意事项：
//   - WarmupScripts 是可选的，不调用也能正常工作（go-redis 会自动处理 NOSCRIPT）
//   - 在 Redis Cluster 模式下，脚本缓存是每个节点独立的，WarmupScripts 只会预热一个节点
//   - 如果应用使用了多个 Redis 客户端（如读写分离），需要对主节点调用 WarmupScripts
//
// # 降级触发策略
//
// 以下情况会触发降级（如果配置了 Fallback 策略）：
//
//   - 网络错误：连接拒绝、连接重置、超时等
//   - Redis Cluster 故障：CLUSTERDOWN、MASTERDOWN、LOADING
//   - 槽迁移失败：MOVED/ASK 错误传到应用层（见下方说明）
//   - 节点只读：READONLY 错误
//
// 以下情况不会触发降级：
//
//   - context.Canceled/DeadlineExceeded：这是客户端超时，不表示 Redis 不可用
//   - TRYAGAIN：这是临时状态，应该由重试机制处理
//   - 业务错误：容量满、配额满等
//
// # 关于 MOVED/ASK 错误
//
// go-redis 库会自动处理 MOVED/ASK 错误并重定向请求。如果这些错误传到应用层，
// 说明重定向也失败了（可能是网络问题或集群配置问题）。在这种情况下触发降级
// 是合理的保护措施，确保服务可用性。
//
// 如果你的场景对全局配额一致性要求极高，不希望在集群迁移期间降级，
// 可以使用 FallbackClose 策略或不配置 Fallback。
//
// 详细使用示例请参考 example_test.go。
package xsemaphore
