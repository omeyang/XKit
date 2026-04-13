package xhealth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// httpGet 向 addr+path 发 GET 请求并返回响应。
func httpGet(t testing.TB, addr, path string) *http.Response {
	t.Helper()
	resp, err := http.Get("http://" + addr + path)
	require.NoError(t, err)
	return resp
}

// readBody 读取响应 body 并关闭。
func readBody(t testing.TB, resp *http.Response) string {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestHTTP_SimpleResponses(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddLivenessCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	require.NoError(t, h.AddReadinessCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	require.NoError(t, h.AddStartupCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/healthz", http.StatusOK, "ok"},
		{"/readyz", http.StatusOK, "ok"},
		{"/startupz", http.StatusOK, "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := httpGet(t, addr, tt.path)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantBody, readBody(t, resp))
		})
	}
}

func TestHTTP_DownResponses(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("fail", CheckConfig{
		Check: func(_ context.Context) error { return errors.New("broken") },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	resp := httpGet(t, addr, "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "not ok", readBody(t, resp))
}

func TestHTTP_DegradedResponses(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("warn", CheckConfig{
		Check:     func(_ context.Context) error { return errors.New("slow") },
		SkipOnErr: true,
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	resp := httpGet(t, addr, "/readyz")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", readBody(t, resp))
}

func TestHTTP_DetailedJSON(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("db", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	require.NoError(t, h.AddReadinessCheck("redis", CheckConfig{
		Check:     func(_ context.Context) error { return errors.New("timeout") },
		SkipOnErr: true,
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	resp := httpGet(t, addr, "/readyz?full=1")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body := readBody(t, resp)
	var result Result
	require.NoError(t, json.Unmarshal([]byte(body), &result))
	assert.Equal(t, StatusDegraded, result.Status)
	assert.Equal(t, StatusUp, result.Checks["db"].Status)
	assert.Equal(t, StatusDegraded, result.Checks["redis"].Status)
	assert.Equal(t, "timeout", result.Checks["redis"].Error)
}

func TestHTTP_SubPath(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("db", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	require.NoError(t, h.AddReadinessCheck("redis", CheckConfig{
		Check: func(_ context.Context) error { return errors.New("down") },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	// 子路径查询单个检查
	resp := httpGet(t, addr, "/readyz/db")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", readBody(t, resp))

	resp = httpGet(t, addr, "/readyz/redis")
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "not ok", readBody(t, resp))

	// 不存在的检查项
	resp = httpGet(t, addr, "/readyz/nonexistent")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	if err := resp.Body.Close(); err != nil {
		return
	}
}

func TestHTTP_SubPath_Detail(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("db", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	resp := httpGet(t, addr, "/readyz/db?full=1")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body := readBody(t, resp)
	var cr CheckResult
	require.NoError(t, json.Unmarshal([]byte(body), &cr))
	assert.Equal(t, StatusUp, cr.Status)
}

func TestHTTP_BasePath(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithBasePath("/api"), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	resp := httpGet(t, addr, "/api/readyz")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", readBody(t, resp))
}

func TestHTTP_ShutdownReturns503(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("ok", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Run(ctx)
	}()
	waitForServer(t, h)

	// Shutdown 标记不健康
	h.Shutdown()
	time.Sleep(50 * time.Millisecond)

	// 服务器已关闭，直接检查 API
	result, err := h.CheckReadiness(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusDown, result.Status)

	cancel()
	require.NoError(t, <-errCh)
}

func TestHTTP_NoChecks(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	// 无检查项时返回 200
	resp := httpGet(t, addr, "/healthz")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", readBody(t, resp))
}

func TestHTTP_CustomDetailParam(t *testing.T) {
	addr := freePort(t)
	h := newTestHealth(t, WithAddr(addr), WithDetailOnQueryParam("verbose"), WithCacheTTL(0))

	require.NoError(t, h.AddReadinessCheck("db", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))

	wait := startHealthInBackground(t, h)
	defer func() {
		require.NoError(t, wait())
	}()

	// 默认 "full" 参数不触发详细响应
	resp := httpGet(t, addr, "/readyz?full=1")
	assert.Equal(t, "ok", readBody(t, resp))

	// 自定义 "verbose" 参数触发 JSON
	resp = httpGet(t, addr, "/readyz?verbose=1")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	if err := resp.Body.Close(); err != nil {
		return
	}
}
