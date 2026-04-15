package xetcd

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// InformerEventType Informer Handler 收到的事件类型。
type InformerEventType int

const (
	// InformerEventPut key 创建或更新。
	InformerEventPut InformerEventType = iota
	// InformerEventDelete key 删除。
	InformerEventDelete
)

// InformerHandler 在每次事件（初始 List 的每个 key、Watch 后续 Put/Delete）被调用。
// 设计决策：Handler 同步执行，调用方应避免阻塞或重活。
// Informer 会 recover Handler 的 panic，避免单个 key 的回调崩溃整条 Watch 流。
type InformerHandler func(eventType InformerEventType, key string, value []byte)

// informerListWatcher 抽象 Informer 使用的 etcd 客户端能力。
// *clientv3.Client 隐式满足此接口；测试可注入 mock 覆盖所有分支。
type informerListWatcher interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
}

// Informer ListWatch 模式：初始全量拉取建立 revision baseline，
// 后续 Watch 增量应用，Watch 故障时用指数退避自动 re-list 恢复。
// 维护一个本地 Store 可供调用方只读访问。
//
// 并发安全：Store 方法并发安全；Informer.Run 应在独立 goroutine 中运行一次。
type Informer struct {
	client  informerListWatcher
	prefix  string
	store   *InformerStore
	handler InformerHandler
	logger  xlog.Logger

	retryBase    time.Duration
	retryMaxWait time.Duration
}

// InformerOption 配置 Informer。
type InformerOption func(*Informer)

// WithInformerHandler 注册事件回调。List 阶段为每个 key 触发一次 Put 事件，
// 后续 Watch 按实际变更触发 Put/Delete。Handler 为 nil 时 Informer 仅维护 Store。
func WithInformerHandler(h InformerHandler) InformerOption {
	return func(i *Informer) { i.handler = h }
}

// WithInformerLogger 注入结构化日志器。nil 时保留默认。
func WithInformerLogger(l xlog.Logger) InformerOption {
	return func(i *Informer) {
		if l != nil {
			i.logger = l
		}
	}
}

// WithInformerBackoff 自定义 re-list 退避区间。下限 <=0 或 上限 <=下限 时静默忽略。
// 默认 (1s, 30s)。
func WithInformerBackoff(base, maxWait time.Duration) InformerOption {
	return func(i *Informer) {
		if base > 0 {
			i.retryBase = base
		}
		if maxWait > 0 && maxWait >= i.retryBase {
			i.retryMaxWait = maxWait
		}
	}
}

const (
	defaultInformerRetryBase    = time.Second
	defaultInformerRetryMaxWait = 30 * time.Second
)

// NewInformer 构造 Informer。prefix 是要监听的 etcd key 前缀（如 "/myapp/"）。
// client 必须是已初始化的 etcd v3 客户端，或任何满足 Get/Watch 语义的 mock。
func NewInformer(client informerListWatcher, prefix string, opts ...InformerOption) *Informer {
	inf := &Informer{
		client:       client,
		prefix:       prefix,
		store:        newInformerStore(),
		logger:       xlog.Default(),
		retryBase:    defaultInformerRetryBase,
		retryMaxWait: defaultInformerRetryMaxWait,
	}
	for _, o := range opts {
		if o != nil {
			o(inf)
		}
	}
	return inf
}

// Store 返回本地缓存的只读视图。Store 本身为值，调用方应将其视作长期句柄。
func (inf *Informer) Store() *InformerStore { return inf.store }

// Run 启动 ListWatch 循环。阻塞直到 ctx 取消或首次 List 失败。
// 首次 List 失败立即返回错误；后续 Watch 故障自动恢复（re-list 退避）。
func (inf *Informer) Run(ctx context.Context) error {
	rev, err := inf.list(ctx)
	if err != nil {
		return fmt.Errorf("xetcd informer: initial list %q: %w", inf.prefix, err)
	}
	return inf.watchLoop(ctx, rev)
}

// list 全量拉取前缀下所有 key-value 并替换 Store；返回 revision。
// 设计决策：re-list 需向 Handler 保证增量契约——
// 对 List 快照中不再存在（即 Watch 断连期间被删除）的旧 key 发 Delete 事件，
// 避免下游状态型消费者漏删。新增/变更的 key 发 Put（值未变的 key 仍发 Put，
// 因为无法可靠判定"是否 re-list 期间发生过短暂变更又还原"，保守重放是安全默认）。
func (inf *Informer) list(ctx context.Context) (int64, error) {
	resp, err := inf.client.Get(ctx, inf.prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, fmt.Errorf("get %q: %w", inf.prefix, err)
	}
	items := make(map[string][]byte, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		items[string(kv.Key)] = kv.Value
	}
	rev := resp.Header.Revision

	// 在 replace 前捕获旧快照，用于 diff 出 Delete 事件（re-list 语义修正）。
	// 首次 List 时 prev 为空，不会发出 Delete。
	var missing []string
	if inf.handler != nil {
		prev := inf.store.List()
		for k := range prev {
			if _, ok := items[k]; !ok {
				missing = append(missing, k)
			}
		}
	}

	inf.store.replace(items, rev)

	if inf.handler != nil {
		for _, k := range missing {
			inf.safeHandle(ctx, InformerEventDelete, k, nil)
		}
		for k, v := range items {
			inf.safeHandle(ctx, InformerEventPut, k, v)
		}
	}
	inf.logger.Info(ctx, "xetcd informer listed",
		slog.String("prefix", inf.prefix),
		slog.Int("count", len(items)),
		slog.Int("deleted_since_last_list", len(missing)),
		slog.Int64("rev", rev))
	return rev, nil
}

// watchLoop 增量监听循环；Watch 故障自动用指数退避 re-list 恢复。
func (inf *Informer) watchLoop(ctx context.Context, startRev int64) error {
	rev := startRev
	backoff := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(inf.retryBase),
		xretry.WithMaxDelay(inf.retryMaxWait),
	)
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		watchErr := inf.watch(ctx, rev+1)
		if watchErr == nil {
			return ctx.Err()
		}
		inf.logger.Warn(ctx, "xetcd informer watch failed, re-listing",
			slog.String("prefix", inf.prefix),
			slog.Int64("rev", rev),
			slog.Any("err", watchErr))

		newRev, listErr := inf.list(ctx)
		if listErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			attempt++
			retryWait := backoff.NextDelay(attempt)
			inf.logger.Warn(ctx, "xetcd informer re-list failed, retrying",
				slog.String("prefix", inf.prefix),
				slog.Any("err", listErr),
				slog.Duration("retry_in", retryWait))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryWait):
			}
			continue
		}
		rev = newRev
		attempt = 0
	}
}

// watch 单次 Watch 执行。返回 nil 表示 ctx 取消，返回 error 表示需要 re-list。
func (inf *Informer) watch(ctx context.Context, rev int64) error {
	wch := inf.client.Watch(ctx, inf.prefix,
		clientv3.WithPrefix(),
		clientv3.WithRev(rev),
		clientv3.WithProgressNotify(),
	)
	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-wch:
			if !ok {
				return fmt.Errorf("watch channel closed")
			}
			if resp.Err() != nil {
				return fmt.Errorf("watch response: %w", resp.Err())
			}
			if resp.IsProgressNotify() {
				continue
			}
			inf.applyEvents(ctx, resp.Events)
		}
	}
}

func (inf *Informer) applyEvents(ctx context.Context, events []*clientv3.Event) {
	for _, ev := range events {
		if ev.Kv == nil {
			inf.logger.Warn(ctx, "xetcd informer: skipping event with nil Kv",
				slog.String("prefix", inf.prefix),
			)
			continue
		}
		key := string(ev.Kv.Key)
		rev := ev.Kv.ModRevision
		switch ev.Type {
		case clientv3.EventTypePut:
			inf.store.set(key, ev.Kv.Value, rev)
			if inf.handler != nil {
				inf.safeHandle(ctx, InformerEventPut, key, ev.Kv.Value)
			}
		case clientv3.EventTypeDelete:
			inf.store.remove(key, rev)
			if inf.handler != nil {
				inf.safeHandle(ctx, InformerEventDelete, key, nil)
			}
		}
	}
}

// safeHandle 调用 Handler 并 recover panic，避免单 key 的回调崩溃整个 Watch。
func (inf *Informer) safeHandle(ctx context.Context, et InformerEventType, key string, value []byte) {
	defer func() {
		if v := recover(); v != nil {
			inf.logger.Error(ctx, "xetcd informer handler panicked",
				slog.String("prefix", inf.prefix),
				slog.String("key", key),
				slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()
	inf.handler(et, key, value)
}
