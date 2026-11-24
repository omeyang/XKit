package xhealth

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoroutineCountCheck(t *testing.T) {
	t.Run("低于阈值通过", func(t *testing.T) {
		check := GoroutineCountCheck(runtime.NumGoroutine() + 1000)
		assert.NoError(t, check(context.Background()))
	})

	t.Run("超过阈值失败", func(t *testing.T) {
		check := GoroutineCountCheck(1)
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds threshold")
	})
}

func TestTCPDialCheck(t *testing.T) {
	t.Run("连接成功", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer func() {
			if err := ln.Close(); err != nil {
				return
			}
		}()

		check := TCPDialCheck(ln.Addr().String())
		assert.NoError(t, check(context.Background()))
	})

	t.Run("连接失败", func(t *testing.T) {
		check := TCPDialCheck("127.0.0.1:1") // 不可达端口
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tcp dial")
	})

	t.Run("context 取消", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		check := TCPDialCheck("127.0.0.1:80")
		err := check(ctx)
		assert.Error(t, err)
	})
}

func TestDatabasePingCheck(t *testing.T) {
	t.Run("nil db", func(t *testing.T) {
		check := DatabasePingCheck(nil)
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database is nil")
	})
}

func TestDNSResolveCheck(t *testing.T) {
	t.Run("有效域名", func(t *testing.T) {
		check := DNSResolveCheck("localhost")
		assert.NoError(t, check(context.Background()))
	})

	t.Run("无效域名", func(t *testing.T) {
		check := DNSResolveCheck("this.domain.does.not.exist.invalid")
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dns resolve")
	})
}

func TestHTTPGetCheck(t *testing.T) {
	t.Run("200 OK", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		check := HTTPGetCheck(ts.URL)
		assert.NoError(t, check(context.Background()))
	})

	t.Run("503 失败", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer ts.Close()

		check := HTTPGetCheck(ts.URL)
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 503")
	})

	t.Run("连接拒绝", func(t *testing.T) {
		check := HTTPGetCheck("http://127.0.0.1:1/test")
		err := check(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "http check")
	})

	t.Run("无效 URL", func(t *testing.T) {
		check := HTTPGetCheck("://invalid")
		err := check(context.Background())
		assert.Error(t, err)
	})
}
