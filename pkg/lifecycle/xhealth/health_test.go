package xhealth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freePort 获取可用端口。
func freePort(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// newTestHealth 创建测试用 Health 实例。
func newTestHealth(t testing.TB, opts ...Option) *Health {
	t.Helper()
	h, err := New(opts...)
	require.NoError(t, err)
	return h
}

// startHealthInBackground 在后台启动 Health，返回等待函数。
func startHealthInBackground(t testing.TB, h *Health) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Run(ctx)
	}()

	// 等待服务器就绪
	waitForServer(t, h)

	return func() error {
		cancel()
		return <-errCh
	}
}

// waitForServer 等待 HTTP 服务启动。
func waitForServer(t testing.TB, h *Health) {
	t.Helper()
	// 等待 readyCh 关闭（listener 就绪信号）
	select {
	case <-h.ReadyCh():
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start in time")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want options
	}{
		{
			name: "默认配置",
			opts: nil,
			want: defaultOptions(),
		},
		{
			name: "自定义地址",
			opts: []Option{WithAddr(":9090")},
			want: func() options {
				o := defaultOptions()
				o.addr = ":9090"
				return o
			}(),
		},
		{
			name: "nil option 被忽略",
			opts: []Option{nil, WithAddr(":9091")},
			want: func() options {
				o := defaultOptions()
				o.addr = ":9091"
				return o
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := New(tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.want.addr, h.opts.addr)
			assert.Equal(t, tt.want.cacheTTL, h.opts.cacheTTL)
			assert.Equal(t, tt.want.detailQueryParam, h.opts.detailQueryParam)
		})
	}
}

func TestAddCheck_Validation(t *testing.T) {
	h := newTestHealth(t)
	goodCheck := CheckConfig{Check: func(_ context.Context) error { return nil }}

	tests := []struct {
		name    string
		check   string
		cfg     CheckConfig
		wantErr error
	}{
		{
			name:    "空名称",
			check:   "",
			cfg:     goodCheck,
			wantErr: ErrEmptyName,
		},
		{
			name:    "nil check",
			check:   "test",
			cfg:     CheckConfig{},
			wantErr: ErrNilCheck,
		},
		{
			name:    "负 interval",
			check:   "test",
			cfg:     CheckConfig{Check: goodCheck.Check, Async: true, Interval: -1},
			wantErr: ErrInvalidInterval,
		},
		{
			name:  "有效检查",
			check: "test",
			cfg:   goodCheck,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 每次用新实例避免重名
			h2 := newTestHealth(t)
			err := h2.AddReadinessCheck(tt.check, tt.cfg)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// 重名检查
	require.NoError(t, h.AddReadinessCheck("db", goodCheck))
	err := h.AddReadinessCheck("db", goodCheck)
	assert.ErrorIs(t, err, ErrDuplicateCheck)
}

func TestAddCheck_AllEndpoints(t *testing.T) {
	h := newTestHealth(t)
	cfg := CheckConfig{Check: func(_ context.Context) error { return nil }}

	require.NoError(t, h.AddLivenessCheck("goroutines", cfg))
	require.NoError(t, h.AddReadinessCheck("db", cfg))
	require.NoError(t, h.AddStartupCheck("init", cfg))

	assert.Equal(t, 1, h.CheckEntries(int(endpointLiveness)))
	assert.Equal(t, 1, h.CheckEntries(int(endpointReadiness)))
	assert.Equal(t, 1, h.CheckEntries(int(endpointStartup)))
}

func TestCheckConfig_Defaults(t *testing.T) {
	cfg := CheckConfig{Check: func(_ context.Context) error { return nil }}
	require.NoError(t, cfg.validate())
	assert.Equal(t, defaultTimeout, cfg.Timeout)

	cfg2 := CheckConfig{
		Check: func(_ context.Context) error { return nil },
		Async: true,
	}
	require.NoError(t, cfg2.validate())
	assert.Equal(t, defaultInterval, cfg2.Interval)
}

func TestCheck_DirectAPI(t *testing.T) {
	h := newTestHealth(t)

	// 无检查项时返回 Up
	result, err := h.CheckLiveness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusUp, result.Status)

	// 添加通过的检查
	require.NoError(t, h.AddReadinessCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	result, err = h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusUp, result.Status)

	// 添加失败的检查（非关键）
	require.NoError(t, h.AddReadinessCheck("degraded", CheckConfig{
		Check:     func(_ context.Context) error { return errors.New("warning") },
		SkipOnErr: true,
	}))
	result, err = h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDegraded, result.Status)

	// 添加失败的关键检查
	require.NoError(t, h.AddReadinessCheck("critical", CheckConfig{
		Check: func(_ context.Context) error { return errors.New("down") },
	}))
	result, err = h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDown, result.Status)
}

func TestCheck_NilContext(t *testing.T) {
	h := newTestHealth(t)

	//nolint:staticcheck // SA1012: 测试 nil context 处理
	_, err := h.CheckLiveness(nil)
	assert.ErrorIs(t, err, ErrNilContext)

	//nolint:staticcheck // SA1012: 测试 nil context 处理
	_, err = h.CheckReadiness(nil)
	assert.ErrorIs(t, err, ErrNilContext)

	//nolint:staticcheck // SA1012: 测试 nil context 处理
	_, err = h.CheckStartup(nil)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestCheck_Timeout(t *testing.T) {
	h := newTestHealth(t, WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("slow", CheckConfig{
		Check: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
		Timeout: 50 * time.Millisecond,
	}))

	result, err := h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDown, result.Status)
	assert.Contains(t, result.Checks["slow"].Error, "context deadline exceeded")
}

func TestCheck_AfterShutdown(t *testing.T) {
	h := newTestHealth(t)
	h.Shutdown()

	result, err := h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDown, result.Status)
}

func TestRun_NilContext(t *testing.T) {
	h := newTestHealth(t)
	//nolint:staticcheck // SA1012: 测试 nil context 处理
	err := h.Run(nil)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestRun_DoubleStart(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	err := h.Run(context.Background())
	assert.ErrorIs(t, err, ErrAlreadyStarted)
}

func TestRun_AfterShutdown(t *testing.T) {
	h := newTestHealth(t)
	h.Shutdown()
	err := h.Run(context.Background())
	assert.ErrorIs(t, err, ErrShutdown)
}

func TestRun_InvalidAddr(t *testing.T) {
	h := newTestHealth(t, WithAddr("invalid-addr-no-port"))
	err := h.Run(context.Background())
	assert.ErrorIs(t, err, ErrInvalidAddr)
}

func TestShutdown_Idempotent(t *testing.T) {
	h := newTestHealth(t)
	h.Shutdown()
	h.Shutdown() // 不应 panic
	assert.True(t, h.IsShutdown())
}

func TestStatusListener(t *testing.T) {
	var mu sync.Mutex
	var changes []string

	listener := func(ep string, old, new Status) {
		mu.Lock()
		defer mu.Unlock()
		changes = append(changes, fmt.Sprintf("%s:%s->%s", ep, old, new))
	}

	h := newTestHealth(t, WithCacheTTL(0), WithStatusListener(listener))

	// 添加失败检查
	require.NoError(t, h.AddReadinessCheck("db", CheckConfig{
		Check: func(_ context.Context) error { return errors.New("fail") },
	}))

	result, err := h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDown, result.Status)

	mu.Lock()
	assert.Contains(t, changes, "readiness:up->down")
	mu.Unlock()
}

func TestAsyncCheck(t *testing.T) {
	var count int
	var mu sync.Mutex

	h := newTestHealth(t, WithAddr(freePort(t)))

	require.NoError(t, h.AddReadinessCheck("async", CheckConfig{
		Check: func(_ context.Context) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		},
		Async:    true,
		Interval: 50 * time.Millisecond,
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	// 等待几次执行
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	c := count
	mu.Unlock()
	assert.Greater(t, c, 1, "异步检查应执行多次")

	result, err := h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusUp, result.Status)
}

func TestSyncCheckCache(t *testing.T) {
	var count int
	var mu sync.Mutex

	h := newTestHealth(t, WithCacheTTL(500*time.Millisecond), WithAddr(freePort(t)))

	require.NoError(t, h.AddReadinessCheck("cached", CheckConfig{
		Check: func(_ context.Context) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		},
	}))

	// 连续调用多次
	for range 5 {
		result, err := h.CheckReadiness(context.Background())
		require.NoError(t, err)
		assert.Equal(t, StatusUp, result.Status)
	}

	mu.Lock()
	c := count
	mu.Unlock()
	// 由于缓存，应只执行 1 次
	assert.Equal(t, 1, c)
}

func TestSyncCheckNoCache(t *testing.T) {
	var count int
	var mu sync.Mutex

	h := newTestHealth(t, WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("uncached", CheckConfig{
		Check: func(_ context.Context) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		},
	}))

	for range 5 {
		result, err := h.CheckReadiness(context.Background())
		require.NoError(t, err)
		assert.Equal(t, StatusUp, result.Status)
	}

	mu.Lock()
	c := count
	mu.Unlock()
	// 无缓存，每次都执行
	assert.Equal(t, 5, c)
}
