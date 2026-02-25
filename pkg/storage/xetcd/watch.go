package xetcd

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EventType 事件类型。
type EventType int

const (
	// EventPut 写入事件。
	EventPut EventType = iota
	// EventDelete 删除事件。
	EventDelete
	// EventUnknown 未知事件类型。
	// 设计决策: 使用显式常量而非默认零值，防止未来 etcd 新增事件类型时
	// 被静默当作 EventPut 处理。
	EventUnknown EventType = -1
)

// String 返回事件类型的字符串表示。
func (e EventType) String() string {
	switch e {
	case EventPut:
		return "PUT"
	case EventDelete:
		return "DELETE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", e)
	}
}

// Event Watch 事件。
// 注意：零值 Event 的 Type 为 EventPut（因为 EventPut = 0）。
// 判断事件有效性应检查 Key 或 Revision 是否非零，而非依赖 Type 字段。
type Event struct {
	// Type 事件类型。
	Type EventType

	// Key 键名。
	Key string

	// Value 键值。Delete 事件时为 nil。
	Value []byte

	// Revision 版本号。
	// 对于正常事件，这是键的修改版本号。
	// 对于错误事件（Error != nil），这是最后成功处理的版本号，
	// 可用于调用 WithRevision(revision+1) 恢复 Watch。
	//
	// ⚠️ 注意：如果 Watch 刚启动就失败（尚未成功处理任何事件），
	// Revision 将为 0。此时无法使用 WithRevision 恢复，
	// 应重新创建 Watch 而不指定起始 revision。
	//
	// 恢复示例：
	//
	//	if event.Error != nil {
	//	    if event.Revision > 0 {
	//	        // 可以从上次成功位置恢复
	//	        ch, _ = client.Watch(ctx, key, WithRevision(event.Revision+1))
	//	    } else {
	//	        // 首次失败，重新开始 Watch
	//	        ch, _ = client.Watch(ctx, key)
	//	    }
	//	}
	Revision int64

	// CompactRevision etcd 数据压缩版本号。
	// 仅在错误事件中有意义（Error != nil）。
	// 当 Watch 因 compaction 失败时，此值表示数据被压缩到的版本。
	// 恢复时应从 max(lastRevision+1, compactRevision) 开始。
	//
	// 值为 0 表示错误不是由 compaction 引起的。
	CompactRevision int64

	// Error Watch 错误。
	// 非 nil 时表示 Watch 失败，Key 和 Value 字段无意义。
	// 接收到错误事件后，通道将被关闭，不会再有后续事件。
	// 此时 Revision 字段包含最后成功处理的版本号，便于恢复。
	// 若 Revision 为 0，表示 Watch 在处理任何事件前就失败了。
	Error error
}

// DefaultWatchBufferSize 默认 Watch 事件通道缓冲区大小。
// 适合中等频率（< 1000 events/s）的场景。
// 高频更新场景建议通过 WithBufferSize 增大此值（如 1024 或更高），
// 低频场景可适当减小以节省内存。
const DefaultWatchBufferSize = 256

// watchOptions Watch 选项。
type watchOptions struct {
	prefix     bool
	revision   int64
	bufferSize int
}

// WatchOption Watch 选项函数。
type WatchOption func(*watchOptions)

// WithPrefix 启用前缀匹配模式，监听指定前缀下所有键的变化。
func WithPrefix() WatchOption {
	return func(o *watchOptions) {
		o.prefix = true
	}
}

// WithRevision 从指定版本开始 Watch，用于恢复 Watch 或获取历史事件。
func WithRevision(rev int64) WatchOption {
	return func(o *watchOptions) {
		o.revision = rev
	}
}

// WithBufferSize 设置事件通道缓冲区大小。
// 默认为 DefaultWatchBufferSize (256)。
// 较大的缓冲区可以减少事件处理慢时的阻塞，但会增加内存占用。
func WithBufferSize(size int) WatchOption {
	return func(o *watchOptions) {
		if size > 0 {
			o.bufferSize = size
		}
	}
}

// Watch 监听键值变化，返回事件通道。
// 通过 context 取消监听，取消时关闭通道。
// 使用 WithPrefix() 监听前缀下所有键的变化。
//
// 事件处理：
//   - 普通事件：Event.Error 为 nil，其他字段有效
//   - 错误事件：Event.Error 非 nil，表示 Watch 失败，通道随后关闭
//
// 重要说明：本方法不自动重连。
// 当发生网络错误或 etcd 集群故障时，会发送错误事件后关闭通道。
// 这是设计决策而非缺陷，原因如下：
//   - 简化错误处理：调用方可以明确知道 Watch 何时失败
//   - 避免隐藏问题：自动重连可能掩盖底层网络或配置问题
//   - 给予控制权：调用方可以决定重连策略（立即重连、退避重连、放弃等）
//
// 调用方应检查 Event.Error 以区分正常事件和 Watch 失败：
//
//	for event := range events {
//	    if event.Error != nil {
//	        // Watch 失败，处理错误
//	        log.Printf("watch error: %v", event.Error)
//	        return
//	    }
//	    // 处理正常事件
//	}
//
// 如需自动重连，可参考以下模式：
//
//	func watchWithRetry(ctx context.Context, client *Client, key string) {
//	    for {
//	        events, err := client.Watch(ctx, key)
//	        if err != nil {
//	            log.Printf("watch failed: %v", err)
//	            return
//	        }
//	        for event := range events {
//	            if event.Error != nil {
//	                log.Printf("watch error: %v, reconnecting...", event.Error)
//	                time.Sleep(time.Second) // 退避
//	                break // 跳出内层循环，重新建立 Watch
//	            }
//	            // 处理正常事件
//	        }
//	        select {
//	        case <-ctx.Done():
//	            return
//	        default:
//	            // 继续重连
//	        }
//	    }
//	}
func (c *Client) Watch(ctx context.Context, key string, opts ...WatchOption) (<-chan Event, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	// 应用选项（使用默认值）
	o := &watchOptions{
		bufferSize: DefaultWatchBufferSize,
	}
	for _, opt := range opts {
		opt(o)
	}

	// 构建 etcd watch 选项
	etcdOpts := c.buildWatchOptions(o)

	// 创建事件通道
	eventCh := make(chan Event, o.bufferSize)

	// 启动 watch goroutine
	c.watchWg.Go(func() {
		c.runWatchLoop(ctx, key, etcdOpts, eventCh)
	})

	return eventCh, nil
}

// buildWatchOptions 构建 etcd watch 选项
func (c *Client) buildWatchOptions(o *watchOptions) []clientv3.OpOption {
	var etcdOpts []clientv3.OpOption
	if o.prefix {
		etcdOpts = append(etcdOpts, clientv3.WithPrefix())
	}
	if o.revision > 0 {
		etcdOpts = append(etcdOpts, clientv3.WithRev(o.revision))
	}
	return etcdOpts
}

// runWatchLoop 运行 watch 循环
func (c *Client) runWatchLoop(ctx context.Context, key string, etcdOpts []clientv3.OpOption, eventCh chan<- Event) {
	defer close(eventCh)

	watchCh := c.client.Watch(ctx, key, etcdOpts...)

	// 跟踪最后一个成功处理的 revision，用于错误恢复
	var lastRevision int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeCh:
			return
		case resp, ok := <-watchCh:
			if !ok {
				// watch 通道被关闭（通常是 context 取消导致）
				return
			}
			if resp.Err() != nil {
				// 发送错误事件，包含最后成功的 revision 和压缩版本号，便于调用方恢复
				// resp.CompactRevision 在 compaction 错误时非零
				c.sendErrorEvent(ctx, eventCh, resp.Err(), lastRevision, resp.CompactRevision)
				return
			}
			var dispatchedRev int64
			dispatchedRev, ok = c.dispatchEvents(ctx, resp.Events, eventCh)
			if !ok {
				return
			}
			if dispatchedRev > 0 {
				lastRevision = dispatchedRev
			}
		}
	}
}

// sendErrorEvent 发送错误事件到通道。
// lastRevision 是最后成功处理的 revision，便于调用方使用 WithRevision 恢复 Watch。
// compactRevision 是 etcd 的压缩版本号，当错误由 compaction 引起时非零。
func (c *Client) sendErrorEvent(ctx context.Context, eventCh chan<- Event, err error, lastRevision, compactRevision int64) {
	select {
	case eventCh <- Event{Error: err, Revision: lastRevision, CompactRevision: compactRevision}:
	case <-ctx.Done():
		// context 已取消，不发送错误事件
	case <-c.closeCh:
		// 客户端已关闭，不发送错误事件
	}
}

// dispatchEvents 分发事件到通道。
// 返回 (lastRevision, ok)，ok 为 false 表示应该退出循环。
func (c *Client) dispatchEvents(ctx context.Context, events []*clientv3.Event, eventCh chan<- Event) (int64, bool) {
	var lastRevision int64
	for _, ev := range events {
		event := convertEvent(ev)
		select {
		case eventCh <- event:
			lastRevision = event.Revision
		case <-ctx.Done():
			return lastRevision, false
		case <-c.closeCh:
			return lastRevision, false
		}
	}
	return lastRevision, true
}

// convertEvent 将 etcd 事件转换为 xetcd 事件。
// 设计决策: 对 ev.Kv 做 nil 守卫，防止网络异常或 etcd 服务端 bug 导致的
// nil Kv 引发 goroutine panic（panic 会绕过 defer close(eventCh)，
// 导致调用方 for range eventCh 永久阻塞）。
func convertEvent(ev *clientv3.Event) Event {
	if ev.Kv == nil {
		return Event{
			Type:  EventUnknown,
			Error: errNilKv,
		}
	}

	event := Event{
		Key:      string(ev.Kv.Key),
		Revision: ev.Kv.ModRevision,
	}

	switch ev.Type {
	case mvccpb.PUT:
		event.Type = EventPut
		event.Value = ev.Kv.Value
	case mvccpb.DELETE:
		event.Type = EventDelete
		event.Value = nil
	default:
		event.Type = EventUnknown
		event.Value = ev.Kv.Value
	}

	return event
}

// =============================================================================
// WatchWithRetry 自动重连支持
// =============================================================================

// RetryConfig Watch 重试配置。
type RetryConfig struct {
	// InitialBackoff 初始退避时间。
	// 默认为 1 秒。
	InitialBackoff time.Duration

	// MaxBackoff 最大退避时间。
	// 默认为 30 秒。
	MaxBackoff time.Duration

	// BackoffMultiplier 退避时间乘数。
	// 默认为 2.0。
	BackoffMultiplier float64

	// MaxRetries 最大重试次数。
	// 0 表示无限重试（直到 context 取消）。
	// 默认为 0（无限重试）。
	MaxRetries int

	// OnRetry 重试时的回调函数。
	// 用于记录日志或监控。
	//
	// ⚠️ 此回调在 WatchWithRetry 内部 goroutine 中被调用，
	// 实现时需注意并发安全（例如使用 atomic 或 sync 保护共享状态）。
	//
	// 参数：
	//   - attempt: 重试次数，从 1 开始
	//   - err: 导致重试的错误
	//   - nextBackoff: 下次退避时间
	//   - lastRevision: 最后成功处理的 revision，可用于日志或恢复确认
	//     值为 0 表示尚未成功处理任何事件（首次连接就失败）
	OnRetry func(attempt int, err error, nextBackoff time.Duration, lastRevision int64)
}

// DefaultRetryConfig 返回默认的重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		MaxRetries:        0, // 无限重试
	}
}

// WatchWithRetry 带自动重连的 Watch。
// 当 Watch 失败时会自动重连，直到 context 取消或达到最大重试次数。
//
// 重连特性：
//   - 使用指数退避策略，避免对 etcd 集群造成压力
//   - 自动从上次成功的 Revision 恢复，尽量避免丢失事件
//   - 支持配置最大重试次数和退避参数
//
// ⚠️ 事件连续性说明：
// 当 Watch 在成功消费任何事件之前就失败（lastRevision=0）时，
// 重连将从 etcd 当前时间点开始，断线窗口内的变更可能丢失。
// 如需严格不丢事件，调用方应通过 WithRevision 指定已知的起始版本号。
//
// 使用示例：
//
//	events, err := client.WatchWithRetry(ctx, "/prefix/",
//	    xetcd.DefaultRetryConfig(),
//	    xetcd.WithPrefix(),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for event := range events {
//	    // 处理事件（不需要检查 Error，重连在内部处理）
//	    fmt.Printf("%s: %s\n", event.Key, event.Value)
//	}
//
// 注意：
//   - 返回的通道只有在 context 取消或达到最大重试次数后才会关闭
//   - 重连期间可能会有短暂的事件延迟
//   - 如果 etcd 集群长时间不可用，事件可能会堆积在 etcd 端
//   - 重连时会自动追加 WithRevision 选项从上次成功位置恢复，
//     调用方传入的 WithRevision 仅在首次连接时生效，不建议同时使用
func (c *Client) WatchWithRetry(ctx context.Context, key string, cfg RetryConfig, opts ...WatchOption) (<-chan Event, error) {
	if err := c.checkPreconditions(ctx); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := validateRetryConfig(&cfg); err != nil {
		return nil, err
	}

	// 创建带缓冲的事件通道
	o := &watchOptions{bufferSize: DefaultWatchBufferSize}
	for _, opt := range opts {
		opt(o)
	}
	eventCh := make(chan Event, o.bufferSize)

	// 启动带重试的 watch goroutine
	c.watchWg.Go(func() {
		c.runWatchWithRetry(ctx, key, cfg, opts, eventCh)
	})

	return eventCh, nil
}

// validateRetryConfig 验证并规范化重试配置。
// 设计决策: 区分零值（使用默认值）和显式负值（配置错误）。
// 零值在 Go 中是结构体的自然初始状态，静默使用默认值提供宽容的 API；
// 显式负值表示调用方的配置逻辑有 bug，应立即报错帮助定位问题。
func validateRetryConfig(cfg *RetryConfig) error {
	if cfg.InitialBackoff < 0 {
		return fmt.Errorf("%w: InitialBackoff must not be negative", ErrInvalidRetryConfig)
	}
	if cfg.MaxBackoff < 0 {
		return fmt.Errorf("%w: MaxBackoff must not be negative", ErrInvalidRetryConfig)
	}
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("%w: MaxRetries must not be negative", ErrInvalidRetryConfig)
	}

	// 应用默认值（零值 → 默认值）
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.BackoffMultiplier < 1 {
		// 设计决策: BackoffMultiplier < 1 意味着退避时间递减（越重试越快），
		// 违反退避策略初衷，静默修正为 2.0 而非返回错误（保持 API 宽容性）。
		cfg.BackoffMultiplier = 2.0
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		// 设计决策: MaxBackoff < InitialBackoff 导致首次退避即被截断，
		// 静默修正为 InitialBackoff，使退避至少从合理值开始。
		cfg.MaxBackoff = cfg.InitialBackoff
	}
	return nil
}

// runWatchWithRetry 运行带重试的 watch 循环。
func (c *Client) runWatchWithRetry(ctx context.Context, key string, cfg RetryConfig, opts []WatchOption, eventCh chan<- Event) {
	defer close(eventCh)

	state := &watchRetryState{
		backoff: cfg.InitialBackoff,
	}

	for {
		if c.shouldStopWatch(ctx) {
			return
		}

		watchOpts := c.buildRetryWatchOptions(opts, state)
		innerCh, err := c.Watch(ctx, key, watchOpts...)
		if err != nil {
			if c.handleWatchRetry(ctx, cfg, state, err) {
				c.sendMaxRetriesErrorIfNeeded(ctx, cfg, state, eventCh)
				return
			}
			continue
		}

		shouldExit, rev, compactRev, disconnectErr, consumed := c.consumeEventsUntilError(ctx, innerCh, eventCh)
		state.updateAfterConsume(rev, compactRev, consumed, cfg.InitialBackoff)
		if shouldExit {
			return
		}

		retryErr := disconnectErrOrDefault(disconnectErr)
		if c.handleWatchRetry(ctx, cfg, state, retryErr) {
			c.sendMaxRetriesErrorIfNeeded(ctx, cfg, state, eventCh)
			return
		}
	}
}

// sendMaxRetriesErrorIfNeeded 在达到最大重试次数时向通道发送错误事件。
// 仅当因 MaxRetries 耗尽而停止时发送，context 取消导致的停止不发送。
func (c *Client) sendMaxRetriesErrorIfNeeded(ctx context.Context, cfg RetryConfig, state *watchRetryState, eventCh chan<- Event) {
	if cfg.MaxRetries > 0 && state.retryCount > cfg.MaxRetries {
		c.sendErrorEvent(ctx, eventCh, ErrMaxRetriesExceeded, state.lastRevision, 0)
	}
}

// updateAfterConsume 更新消费事件后的重试状态。
// 成功消费过事件说明连接曾正常，重置连续失败计数。
func (s *watchRetryState) updateAfterConsume(rev, compactRev int64, consumed bool, initialBackoff time.Duration) {
	if rev > 0 {
		s.lastRevision = rev
	}
	if compactRev > 0 {
		s.compactRevision = compactRev
	}
	if consumed {
		s.retryCount = 0
		s.backoff = initialBackoff
	}
}

// disconnectErrOrDefault 返回断开错误，无错误时使用 ErrWatchDisconnected 哨兵错误。
func disconnectErrOrDefault(err error) error {
	if err != nil {
		return err
	}
	return ErrWatchDisconnected
}

// watchRetryState 保存 watch 重试状态。
type watchRetryState struct {
	lastRevision    int64
	compactRevision int64 // 最近一次 compaction 错误的压缩版本号
	backoff         time.Duration
	retryCount      int
}

// shouldStopWatch 检查是否应停止 watch。
func (c *Client) shouldStopWatch(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}
	return c.isClosed()
}

// buildRetryWatchOptions 构建重试 watch 的选项，根据状态从合适的 revision 恢复。
// 当发生 compaction 错误时，会使用 max(lastRevision+1, compactRevision) 作为起始版本。
func (c *Client) buildRetryWatchOptions(opts []WatchOption, state *watchRetryState) []WatchOption {
	watchOpts := make([]WatchOption, len(opts))
	copy(watchOpts, opts)

	// 计算恢复的起始 revision
	// 正常情况：从 lastRevision+1 开始
	// compaction 情况：如果 lastRevision+1 < compactRevision，需要从 compactRevision 开始
	// 因为已压缩的版本无法 watch
	startRev := state.lastRevision + 1
	if state.compactRevision > 0 && startRev < state.compactRevision {
		startRev = state.compactRevision
	}

	if startRev > 1 { // startRev > 1 表示有有效的恢复点（lastRevision > 0 或有 compaction）
		watchOpts = append(watchOpts, WithRevision(startRev))
	}
	return watchOpts
}

// handleWatchRetry 处理 watch 重试逻辑。
// 返回 true 表示应该停止重试。
func (c *Client) handleWatchRetry(ctx context.Context, cfg RetryConfig, state *watchRetryState, err error) bool {
	state.retryCount++
	if cfg.MaxRetries > 0 && state.retryCount > cfg.MaxRetries {
		return true
	}
	// 在执行回调和 sleep 前检查 context，避免已取消后仍执行耗时操作
	select {
	case <-ctx.Done():
		return true
	default:
	}
	if cfg.OnRetry != nil {
		cfg.OnRetry(state.retryCount, err, state.backoff, state.lastRevision)
	}
	sleepWithCancel(ctx, state.backoff, c.closeCh)
	state.backoff = nextBackoff(state.backoff, cfg)
	return false
}

// consumeEventsUntilError 消费事件直到发生错误。
// 返回 (shouldExit, lastRevision, compactRevision, disconnectErr)。
// compactRevision 在 compaction 错误时非零，用于恢复时跳过已压缩的版本。
// disconnectErr 是导致断开的原始错误，用于传递给 OnRetry 回调。
// eventsConsumed 表示是否成功消费了至少一个正常事件。
func (c *Client) consumeEventsUntilError(ctx context.Context, innerCh <-chan Event, eventCh chan<- Event) (shouldExit bool, lastRevision int64, compactRevision int64, disconnectErr error, eventsConsumed bool) {
	for {
		select {
		case <-ctx.Done():
			return true, lastRevision, 0, nil, eventsConsumed
		case <-c.closeCh:
			return true, lastRevision, 0, nil, eventsConsumed
		case event, ok := <-innerCh:
			if !ok {
				// 通道关闭，需要重连
				return false, lastRevision, 0, nil, eventsConsumed
			}
			if event.Error != nil {
				// 发生错误，需要重连
				if event.Revision > 0 {
					lastRevision = event.Revision
				}
				return false, lastRevision, event.CompactRevision, event.Error, eventsConsumed
			}
			// 正常事件，转发到输出通道
			eventsConsumed = true
			lastRevision = event.Revision
			if !c.forwardEvent(ctx, eventCh, event) {
				return true, lastRevision, 0, nil, eventsConsumed
			}
		}
	}
}

// forwardEvent 将事件转发到输出通道。
// 返回 false 表示应该退出（context 取消或客户端关闭）。
func (c *Client) forwardEvent(ctx context.Context, eventCh chan<- Event, event Event) bool {
	select {
	case eventCh <- event:
		return true
	case <-ctx.Done():
		return false
	case <-c.closeCh:
		return false
	}
}

// sleepWithCancel 带 context 和关闭信号的 sleep。
// 当 ctx 取消、done 通道关闭或定时器到期时返回，
// 确保 Close() 期间退避睡眠能被及时中断。
func sleepWithCancel(ctx context.Context, d time.Duration, done <-chan struct{}) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-done:
	case <-timer.C:
	}
}

// nextBackoff 计算下一次退避时间。
// 设计决策: 添加 ±20% 随机抖动（jitter）防止惊群效应。
// 当多个客户端同时丢失连接（如 etcd 集群重启）时，确定性退避会导致
// 所有客户端在相同时间点重连，对刚恢复的集群造成突发压力。
func nextBackoff(current time.Duration, cfg RetryConfig) time.Duration {
	next := time.Duration(float64(current) * cfg.BackoffMultiplier)
	next = addJitter(next)
	return min(next, cfg.MaxBackoff)
}

// jitter 随机数常量，与 xretry 保持一致。
// 设计决策: 此处与 pkg/resilience/xretry/backoff.go 中的 randomFloat64 实现相同算法。
// 有意保持 copy-paste 而非提取到 internal/ 共享包，原因：
//   - xetcd 与 xretry 属于不同领域模块，避免引入跨域依赖
//   - 算法稳定（crypto/rand + bit-shift 生成 [0,1) 浮点数），变更概率极低
//   - 若需修改，两处代码均有注释指向对方，便于同步
const (
	jitterFloatBits  = 53
	jitterFloatScale = 1.0 / (1 << jitterFloatBits)
)

// addJitter 为退避时间添加 ±20% 的随机抖动。
// 使用 crypto/rand 确保高质量随机数（与 xretry 保持一致）。
func addJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	// 生成 [0, 1) 的随机浮点数
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 设计决策: crypto/rand 失败时返回无抖动（安全默认值），而非 panic 或降级到 math/rand。
		// Go 1.22+ 中 crypto/rand.Read 在支持的平台上永不失败（使用 getrandom/getentropy），
		// 此分支仅为防御极端情况（如 /dev/urandom 不可用的受限环境）。
		// 与 xretry 包的 randomFloat64 保持一致的 fallback 策略。
		return d
	}
	r := float64(binary.LittleEndian.Uint64(buf[:])>>11) * jitterFloatScale
	// r ∈ [0, 1)，映射到 [-0.2, +0.2) 得到 ±20% 抖动
	jitter := time.Duration(float64(d) * (r*0.4 - 0.2))
	return d + jitter
}
