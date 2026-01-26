package xlimit

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

// mockFailingLimiter 模拟 Redis 故障的限流器
type mockFailingLimiter struct {
	failOnAllow bool
	failOnReset bool
	failErr     error
}

func (m *mockFailingLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
	return m.AllowN(ctx, key, 1)
}

func (m *mockFailingLimiter) AllowN(_ context.Context, _ Key, _ int) (*Result, error) {
	if m.failOnAllow {
		return nil, m.failErr
	}
	return &Result{Allowed: true, Limit: 100, Remaining: 99}, nil
}

func (m *mockFailingLimiter) Reset(_ context.Context, _ Key) error {
	if m.failOnReset {
		return m.failErr
	}
	return nil
}

func (m *mockFailingLimiter) Close() error {
	return nil
}

func (m *mockFailingLimiter) Query(_ context.Context, _ Key) (*QuotaInfo, error) {
	if m.failOnAllow {
		return nil, m.failErr
	}
	return &QuotaInfo{Limit: 100, Remaining: 99, Rule: "mock"}, nil
}

func TestFallbackLimiter_NoFallback(t *testing.T) {
	// 当分布式限流正常工作时，不应该触发降级
	distributed := &mockFailingLimiter{failOnAllow: false}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "test-tenant"}

	result, err := fallback.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed")
	}
	if result.Remaining != 99 {
		t.Errorf("expected remaining 99 (from mock), got %d", result.Remaining)
	}
}

func TestFallbackLimiter_FallbackToLocal(t *testing.T) {
	// 当 Redis 故障时，降级到本地限流
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 5, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "fallback-tenant"}

	// 应该使用本地限流
	result, err := fallback.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error on fallback, got %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed from local limiter")
	}
	// 本地限流器应该返回正确的 remaining
	if result.Remaining < 0 || result.Remaining > 5 {
		t.Errorf("unexpected remaining %d", result.Remaining)
	}
}

func TestFallbackLimiter_FallbackOpen(t *testing.T) {
	// fail-open 策略：Redis 故障时放行所有请求
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 1, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackOpen}})

	ctx := context.Background()
	key := Key{Tenant: "open-tenant"}

	// 即使本地限流器配额耗尽，fail-open 也应该放行
	for i := range 10 {
		result, err := fallback.Allow(ctx, key)
		if err != nil {
			t.Fatalf("expected no error on fallback-open, got %v", err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed with fail-open", i+1)
		}
		if result.Rule != "fallback-open" {
			t.Errorf("expected rule 'fallback-open', got %q", result.Rule)
		}
	}
}

func TestFallbackLimiter_FallbackClose(t *testing.T) {
	// fail-close 策略：Redis 故障时拒绝所有请求
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 100, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackClose}})

	ctx := context.Background()
	key := Key{Tenant: "close-tenant"}

	result, err := fallback.Allow(ctx, key)
	if err == nil {
		t.Fatal("expected error on fallback-close")
	}
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected ErrRedisUnavailable, got %v", err)
	}
	if result.Allowed {
		t.Error("should not be allowed with fail-close")
	}
	if result.Rule != "fallback-close" {
		t.Errorf("expected rule 'fallback-close', got %q", result.Rule)
	}
}

func TestFallbackLimiter_NonRedisError(t *testing.T) {
	// 非 Redis 错误不应该触发降级
	nonRedisErr := errors.New("some other error")
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     nonRedisErr,
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "error-tenant"}

	_, err = fallback.Allow(ctx, key)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if err != nonRedisErr {
		t.Errorf("expected original error, got %v", err)
	}
}

func TestFallbackLimiter_AllowN(t *testing.T) {
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "batch-tenant"}

	// 批量请求也应该降级到本地
	result, err := fallback.AllowN(ctx, key, 5)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed")
	}
}

func TestFallbackLimiter_Reset(t *testing.T) {
	distributed := &mockFailingLimiter{failOnReset: false}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "reset-tenant"}

	// 重置应该同时重置分布式和本地
	err = fallback.Reset(ctx, key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestFallbackLimiter_ResetWithRedisError(t *testing.T) {
	// Redis 故障时 Reset 应该忽略 Redis 错误并继续重置本地
	distributed := &mockFailingLimiter{
		failOnReset: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "reset-fallback-tenant"}

	// Redis 错误应该被忽略
	err = fallback.Reset(ctx, key)
	if err != nil {
		t.Fatalf("expected Redis error to be ignored, got %v", err)
	}
}

func TestFallbackLimiter_Close(t *testing.T) {
	distributed := &mockFailingLimiter{}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	err = fallback.Close()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestIsRedisError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"syscall ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"syscall ECONNRESET", syscall.ECONNRESET, true},
		{"syscall ETIMEDOUT", syscall.ETIMEDOUT, true},
		{"syscall EPIPE", syscall.EPIPE, true},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"net.OpError", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, true},
		{"net.DNSError", &net.DNSError{Err: "lookup failed"}, true},
		{"ErrRedisUnavailable", ErrRedisUnavailable, true},
		{"other error", errors.New("some other error"), false},
		{"validation error", ErrInvalidRule, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsRedisError(tc.err)
			if result != tc.expected {
				t.Errorf("IsRedisError(%v) = %v, expected %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestFallbackLimiter_DefaultStrategy(t *testing.T) {
	// 测试默认策略（非法策略应该回退到本地）
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED, // 使用真实的连接错误类型
	}
	local, err := NewLocal(WithRules(TenantRule("test", 5, time.Minute)))
	if err != nil {
		t.Fatalf("NewLocal failed: %v", err)
	}
	defer func() { _ = local.Close() }() //nolint:errcheck // defer cleanup

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackStrategy("unknown")}})

	ctx := context.Background()
	key := Key{Tenant: "default-tenant"}

	// 未知策略应该默认使用本地限流
	result, err := fallback.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error on default fallback, got %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed from local limiter")
	}
}

// mockCloseLimiter 模拟 Close 失败的限流器
type mockCloseLimiter struct {
	mockFailingLimiter
	failOnClose bool
	closeErr    error
}

func (m *mockCloseLimiter) Close() error {
	if m.failOnClose {
		return m.closeErr
	}
	return nil
}

func TestFallbackLimiter_CloseWithErrors(t *testing.T) {
	// 测试 Close 时两个限流器都失败的情况
	closeErr := errors.New("close error")
	distributed := &mockCloseLimiter{
		failOnClose: true,
		closeErr:    closeErr,
	}
	local := &mockCloseLimiter{
		failOnClose: true,
		closeErr:    closeErr,
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	err := fallback.Close()
	if err == nil {
		t.Fatal("expected error when both limiters fail to close")
	}
}

func TestFallbackLimiter_CloseWithDistributedError(t *testing.T) {
	// 测试 Close 时只有分布式限流器失败
	closeErr := errors.New("distributed close error")
	distributed := &mockCloseLimiter{
		failOnClose: true,
		closeErr:    closeErr,
	}
	local := &mockCloseLimiter{
		failOnClose: false,
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	err := fallback.Close()
	if err == nil {
		t.Fatal("expected error when distributed limiter fails to close")
	}
}

func TestFallbackLimiter_CloseWithLocalError(t *testing.T) {
	// 测试 Close 时只有本地限流器失败
	closeErr := errors.New("local close error")
	distributed := &mockCloseLimiter{
		failOnClose: false,
	}
	local := &mockCloseLimiter{
		failOnClose: true,
		closeErr:    closeErr,
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	err := fallback.Close()
	if err == nil {
		t.Fatal("expected error when local limiter fails to close")
	}
}

// mockResetLimiter 模拟 Reset 可以失败的限流器
type mockResetLimiter struct {
	mockFailingLimiter
	resetErr error
}

func (m *mockResetLimiter) Reset(_ context.Context, _ Key) error {
	return m.resetErr
}

func TestFallbackLimiter_ResetWithLocalError(t *testing.T) {
	// 测试 Reset 时本地限流器失败
	resetErr := errors.New("local reset error")
	distributed := &mockResetLimiter{
		resetErr: nil, // 分布式成功
	}
	local := &mockResetLimiter{
		resetErr: resetErr, // 本地失败
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "reset-error-tenant"}

	err := fallback.Reset(ctx, key)
	if err == nil {
		t.Fatal("expected error when local limiter fails to reset")
	}
}

func TestFallbackLimiter_ResetWithNonRedisDistributedError(t *testing.T) {
	// 测试 Reset 时分布式限流器返回非 Redis 错误
	nonRedisErr := errors.New("non-redis error")
	distributed := &mockResetLimiter{
		resetErr: nonRedisErr,
	}
	local := &mockResetLimiter{
		resetErr: nil,
	}

	fallback := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})

	ctx := context.Background()
	key := Key{Tenant: "reset-non-redis-tenant"}

	err := fallback.Reset(ctx, key)
	if err == nil {
		t.Fatal("expected error when distributed limiter returns non-Redis error")
	}
}
