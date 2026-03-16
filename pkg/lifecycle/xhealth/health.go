package xhealth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// endpoint 标识健康检查端点类型。
type endpoint int

const (
	endpointLiveness endpoint = iota
	endpointReadiness
	endpointStartup
)

// endpointNames 用于日志和回调中的端点名称。
var endpointNames = [...]string{
	endpointLiveness:  "liveness",
	endpointReadiness: "readiness",
	endpointStartup:   "startup",
}

// Health 管理健康检查的主结构体。
//
// 通过 [New] 创建实例，注册检查项后调用 [Health.Run] 启动 HTTP 服务。
// 实现了 xrun.Service 接口。
type Health struct {
	opts options

	mu       sync.RWMutex
	checks   [3][]*checkEntry // 按 endpoint 索引
	statuses [3]Status        // 各端点的当前聚合状态

	started  atomic.Bool
	shutdown atomic.Bool

	server   atomic.Pointer[http.Server]
	stopOnce sync.Once
	stopCh   chan struct{} // 通知异步 goroutine 停止
	readyCh  chan struct{} // Run 中 listener 就绪后关闭
}

// New 创建健康检查实例。
//
// 示例：
//
//	h, err := xhealth.New(
//	    xhealth.WithAddr(":8081"),
//	    xhealth.WithCacheTTL(2 * time.Second),
//	)
func New(opts ...Option) (*Health, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	h := &Health{
		opts:    o,
		stopCh:  make(chan struct{}),
		readyCh: make(chan struct{}),
	}
	for i := range h.statuses {
		h.statuses[i] = StatusUp
	}

	return h, nil
}

// AddLivenessCheck 注册 liveness 检查项。
//
// Liveness 检查用于检测进程死锁等内部故障，不应检查外部依赖
// （如数据库、Redis），否则外部故障会导致 Pod 重启引发级联故障。
func (h *Health) AddLivenessCheck(name string, cfg CheckConfig) error {
	return h.addCheck(endpointLiveness, name, cfg)
}

// AddReadinessCheck 注册 readiness 检查项。
//
// Readiness 检查用于判断服务是否能接受流量。
// 适合检查外部依赖（数据库连接、缓存可用性等）。
func (h *Health) AddReadinessCheck(name string, cfg CheckConfig) error {
	return h.addCheck(endpointReadiness, name, cfg)
}

// AddStartupCheck 注册 startup 检查项。
//
// Startup 检查用于判断服务是否完成启动。
// K8s 会在 startup 检查通过后才开始执行 liveness 和 readiness 检查。
func (h *Health) AddStartupCheck(name string, cfg CheckConfig) error {
	return h.addCheck(endpointStartup, name, cfg)
}

// addCheck 将检查项注册到指定端点。
func (h *Health) addCheck(ep endpoint, name string, cfg CheckConfig) error {
	if name == "" {
		return ErrEmptyName
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, e := range h.checks[ep] {
		if e.name == name {
			return fmt.Errorf("%w: %s", ErrDuplicateCheck, name)
		}
	}

	h.checks[ep] = append(h.checks[ep], &checkEntry{
		name:   name,
		config: cfg,
	})
	return nil
}

// Run 启动 HTTP 健康检查服务，阻塞直到 ctx 被取消或发生错误。
//
// 实现 xrun.Service 接口。当 ctx 被取消时，服务会优雅关闭。
func (h *Health) Run(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if h.shutdown.Load() {
		return ErrShutdown
	}
	if !h.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	// 启动异步检查的后台 goroutine
	var wg sync.WaitGroup
	h.startAsyncChecks(&wg)

	mux := http.NewServeMux()
	h.registerHandlers(mux)

	ln, err := net.Listen("tcp", h.opts.addr)
	if err != nil {
		h.started.Store(false)
		return fmt.Errorf("%w: %v", ErrInvalidAddr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	h.server.Store(srv)
	close(h.readyCh) // 通知 listener 就绪

	// 监听取消信号
	go func() {
		select {
		case <-ctx.Done():
			h.doShutdown()
		case <-h.stopCh:
		}
	}()

	err = srv.Serve(ln)
	// 等待异步 goroutine 退出
	wg.Wait()

	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown 标记所有端点为不健康并停止 HTTP 服务。
//
// 调用后所有探针请求立即返回 503，配合 K8s 流量摘除。
// Shutdown 是幂等的，可安全多次调用。
func (h *Health) Shutdown() {
	h.doShutdown()
}

// doShutdown 执行实际关闭逻辑（幂等）。
func (h *Health) doShutdown() {
	h.stopOnce.Do(func() {
		h.shutdown.Store(true)
		close(h.stopCh)
		if srv := h.server.Load(); srv != nil {
			if err := srv.Shutdown(context.Background()); err != nil {
				return
			}
		}
	})
}

// CheckLiveness 直接获取 liveness 检查结果（不经过 HTTP）。
func (h *Health) CheckLiveness(ctx context.Context) (*Result, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return h.check(ctx, endpointLiveness), nil
}

// CheckReadiness 直接获取 readiness 检查结果（不经过 HTTP）。
func (h *Health) CheckReadiness(ctx context.Context) (*Result, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return h.check(ctx, endpointReadiness), nil
}

// CheckStartup 直接获取 startup 检查结果（不经过 HTTP）。
func (h *Health) CheckStartup(ctx context.Context) (*Result, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return h.check(ctx, endpointStartup), nil
}

// check 执行指定端点的所有检查并返回聚合结果。
func (h *Health) check(ctx context.Context, ep endpoint) *Result {
	if h.shutdown.Load() {
		return &Result{Status: StatusDown}
	}

	h.mu.RLock()
	entries := h.checks[ep]
	h.mu.RUnlock()

	if len(entries) == 0 {
		return &Result{Status: StatusUp}
	}

	results := make(map[string]CheckResult, len(entries))
	for _, entry := range entries {
		results[entry.name] = h.executeCheck(ctx, entry)
	}

	result := aggregate(entries, results)
	h.maybeNotifyStatusChange(ep, result.Status)
	return result
}

// executeCheck 执行单个检查：异步检查返回缓存，同步检查可选缓存。
func (h *Health) executeCheck(ctx context.Context, entry *checkEntry) CheckResult {
	if entry.config.Async {
		if cached, ok := entry.getCached(); ok {
			return cached
		}
		// 异步检查尚未执行，返回默认 up
		return CheckResult{Status: StatusUp}
	}

	// 同步检查：检查缓存
	if h.opts.cacheTTL > 0 {
		if cached, ok := entry.getCached(); ok {
			return cached
		}
	}

	result := entry.execute(ctx)

	if h.opts.cacheTTL > 0 {
		entry.setCachedWithTTL(result, h.opts.cacheTTL)
	}

	return result
}

// startAsyncChecks 启动所有异步检查的后台 goroutine。
func (h *Health) startAsyncChecks(wg *sync.WaitGroup) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for epIdx := range h.checks {
		for _, entry := range h.checks[epIdx] {
			if !entry.config.Async {
				continue
			}
			wg.Add(1)
			go h.runAsyncCheck(wg, entry)
		}
	}
}

// runAsyncCheck 后台定期执行异步检查。
func (h *Health) runAsyncCheck(wg *sync.WaitGroup, entry *checkEntry) {
	defer wg.Done()

	// 立即执行一次
	result := entry.execute(context.Background())
	entry.setCached(result)

	ticker := time.NewTicker(entry.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			result := entry.execute(context.Background())
			entry.setCached(result)
		case <-h.stopCh:
			return
		}
	}
}

// maybeNotifyStatusChange 在状态变化时调用回调。
func (h *Health) maybeNotifyStatusChange(ep endpoint, newStatus Status) {
	if h.opts.statusListener == nil {
		return
	}

	h.mu.Lock()
	oldStatus := h.statuses[ep]
	if oldStatus == newStatus {
		h.mu.Unlock()
		return
	}
	h.statuses[ep] = newStatus
	h.mu.Unlock()

	h.opts.statusListener(endpointNames[ep], oldStatus, newStatus)
}
