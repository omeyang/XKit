package xhealth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// CheckFunc 定义健康检查函数签名。
//
// 返回 nil 表示检查通过，返回 error 表示检查失败。
// ctx 携带超时控制，实现方应尊重 ctx.Done() 信号。
type CheckFunc func(ctx context.Context) error

// CheckConfig 定义健康检查项的配置。
type CheckConfig struct {
	// Check 是检查函数，不能为 nil。
	Check CheckFunc

	// Timeout 是单次检查的超时时间，默认 5s。
	// 设为 0 表示使用默认值。
	Timeout time.Duration

	// SkipOnErr 标记此检查项为非关键：失败时状态为 Degraded 而非 Down。
	SkipOnErr bool

	// Async 启用异步检查模式：后台定期执行，探针请求读取缓存结果。
	Async bool

	// Interval 是异步检查的执行间隔，仅在 Async=true 时有效，默认 10s。
	// 设为 0 表示使用默认值。
	Interval time.Duration
}

const (
	defaultTimeout  = 5 * time.Second
	defaultInterval = 10 * time.Second
)

// validate 校验 CheckConfig 并填充默认值。
func (c *CheckConfig) validate() error {
	if c.Check == nil {
		return ErrNilCheck
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.Timeout < 0 {
		return fmt.Errorf("%w: got %v", ErrInvalidTimeout, c.Timeout)
	}
	if c.Async {
		if c.Interval == 0 {
			c.Interval = defaultInterval
		}
		if c.Interval < 0 {
			return fmt.Errorf("%w: got %v", ErrInvalidInterval, c.Interval)
		}
	}
	return nil
}

// checkEntry 是注册到端点的检查项。
type checkEntry struct {
	name   string
	config CheckConfig

	// 缓存字段（异步检查和同步缓存共用）
	mu        sync.RWMutex
	cached    *CheckResult
	expiresAt time.Time // 缓存过期时间（零值表示不过期，用于异步检查）

	// sf 保证同步缓存 miss/过期时同一 checkEntry 只有一个执行中的 Check,
	// 避免 TTL 失效/冷启动时并发探针惊群放大下游依赖压力。
	sf singleflight.Group
}

// getCached 返回缓存的检查结果（线程安全）。
// 如果缓存已过期则返回 false。
func (e *checkEntry) getCached() (CheckResult, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.cached == nil {
		return CheckResult{}, false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		return CheckResult{}, false
	}
	return *e.cached, true
}

// setCached 设置缓存的检查结果（线程安全）。
func (e *checkEntry) setCached(r CheckResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cached = &r
}

// setCachedWithTTL 设置缓存的检查结果并设定过期时间（线程安全）。
func (e *checkEntry) setCachedWithTTL(r CheckResult, ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cached = &r
	e.expiresAt = time.Now().Add(ttl)
}

// execute 执行检查并返回结果。
func (e *checkEntry) execute(ctx context.Context) CheckResult {
	checkCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	start := time.Now()
	err := e.config.Check(checkCtx)
	duration := time.Since(start)

	if err != nil {
		status := StatusDown
		if e.config.SkipOnErr {
			status = StatusDegraded
		}
		return CheckResult{
			Status:   status,
			Error:    err.Error(),
			Duration: duration,
		}
	}
	return CheckResult{
		Status:   StatusUp,
		Duration: duration,
	}
}

// aggregate 聚合多个检查项的结果为一个 Result。
func aggregate(entries []*checkEntry, results map[string]CheckResult) *Result {
	r := &Result{
		Status: StatusUp,
		Checks: results,
	}

	for _, entry := range entries {
		cr, ok := results[entry.name]
		if !ok {
			continue
		}
		switch {
		case cr.Status == StatusDown && r.Status != StatusDown:
			r.Status = StatusDown
		case cr.Status == StatusDegraded && r.Status == StatusUp:
			r.Status = StatusDegraded
		}
	}

	return r
}
