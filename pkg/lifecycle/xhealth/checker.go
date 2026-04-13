package xhealth

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"runtime"
)

// GoroutineCountCheck 返回检查 goroutine 数量的函数。
//
// 当 goroutine 数量超过 threshold 时返回错误。
// 适合 liveness 检查，用于检测 goroutine 泄漏。
func GoroutineCountCheck(threshold int) CheckFunc {
	return func(_ context.Context) error {
		count := runtime.NumGoroutine()
		if count > threshold {
			return fmt.Errorf(
				"goroutine count %d exceeds threshold %d",
				count, threshold,
			)
		}
		return nil
	}
}

// TCPDialCheck 返回检查 TCP 连接的函数。
//
// 尝试连接 addr（格式 "host:port"），连接成功表示检查通过。
// 超时由 ctx 控制。
func TCPDialCheck(addr string) CheckFunc {
	return func(ctx context.Context) error {
		d := net.Dialer{}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("tcp dial %s: %w", addr, err)
		}
		return conn.Close()
	}
}

// DatabasePingCheck 返回检查数据库连接的函数。
//
// 使用 sql.DB.PingContext 检查连接池是否可用。
func DatabasePingCheck(db *sql.DB) CheckFunc {
	return func(ctx context.Context) error {
		if db == nil {
			return fmt.Errorf("database is nil")
		}
		return db.PingContext(ctx)
	}
}

// DNSResolveCheck 返回检查 DNS 解析的函数。
//
// 尝试解析 host，成功解析出至少一个地址表示检查通过。
func DNSResolveCheck(host string) CheckFunc {
	return func(ctx context.Context) error {
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return fmt.Errorf("dns resolve %s: %w", host, err)
		}
		if len(addrs) == 0 {
			return fmt.Errorf("dns resolve %s: no addresses found", host)
		}
		return nil
	}
}

// HTTPGetCheck 返回检查 HTTP GET 请求的函数。
//
// 向 url 发送 GET 请求，HTTP 2xx 表示检查通过。
// 超时由 ctx 控制。
func HTTPGetCheck(url string) CheckFunc {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("http check %s: %w", url, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("http check %s: %w", url, err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				return
			}
		}()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf(
				"http check %s: unexpected status %d",
				url, resp.StatusCode,
			)
		}
		return nil
	}
}
