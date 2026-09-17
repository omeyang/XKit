package rediscompat

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ScriptMode 类型测试
// ============================================================================

func TestScriptMode_String(t *testing.T) {
	tests := []struct {
		mode ScriptMode
		want string
	}{
		{ScriptModeAuto, "auto"},
		{ScriptModeLua, "lua"},
		{ScriptModeCompat, "compat"},
		{ScriptMode(99), "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.mode.String())
	}
}

func TestScriptMode_IsValid(t *testing.T) {
	tests := []struct {
		mode ScriptMode
		want bool
	}{
		{ScriptModeAuto, true},
		{ScriptModeLua, true},
		{ScriptModeCompat, true},
		{ScriptMode(-1), false},
		{ScriptMode(99), false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.mode.IsValid(), "mode=%d", tt.mode)
	}
}

// ============================================================================
// DetectScriptMode 测试
// ============================================================================

func TestDetectScriptMode(t *testing.T) {
	t.Run("支持 Lua 脚本", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		mode, err := DetectScriptMode(context.Background(), client)
		assert.NoError(t, err)
		assert.Equal(t, ScriptModeLua, mode)
	})

	t.Run("网络错误返回 Lua 模式和 error", func(t *testing.T) {
		// 连接一个不存在的地址
		client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
		defer client.Close()

		mode, err := DetectScriptMode(context.Background(), client)
		assert.Error(t, err)
		assert.Equal(t, ScriptModeLua, mode)
	})

	t.Run("代理不支持 EVAL 返回 Compat", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		// 模拟代理: 拦截 EVAL 命令返回权限错误
		mr.SetError("ERR auth permission deny")

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		mode, detectErr := DetectScriptMode(context.Background(), client)
		assert.NoError(t, detectErr)
		assert.Equal(t, ScriptModeCompat, mode)

		mr.SetError("") // 清除错误
	})
}

// ============================================================================
// DetectScriptModeBounded 测试（构造函数防黑洞场景）
// ============================================================================

// TestDefaultDetectTimeout_Value 锁定默认 5s，被改动时必须显式更新本测试 + memory。
func TestDefaultDetectTimeout_Value(t *testing.T) {
	assert.Equal(t, 5*time.Second, DefaultDetectTimeout)
	assert.Equal(t, DefaultDetectTimeout, detectTimeout, "包内可变副本默认与常量同值")
}

func TestDetectScriptModeBounded(t *testing.T) {
	t.Run("正常 Redis：返回 Lua 模式", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		mode, err := DetectScriptModeBounded(client)
		assert.NoError(t, err)
		assert.Equal(t, ScriptModeLua, mode)
	})

	t.Run("黑洞地址：超时内有界返回，不卡死构造函数", func(t *testing.T) {
		// 启监听器但 accept 后永不读，让 Redis 客户端 dial 成功后阻塞在等响应阶段。
		// 这模拟"TCP 连得通但 Redis 协议层无回应"的黑洞代理/防火墙场景。
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer func() { _ = ln.Close() }() //nolint:errcheck // test cleanup

		done := make(chan struct{})
		defer close(done)
		go func() {
			for {
				conn, acceptErr := ln.Accept()
				if acceptErr != nil {
					return
				}
				// 持有连接但永不读/不回，让客户端挂在 read 上
				go func(c net.Conn) {
					<-done
					_ = c.Close() //nolint:errcheck // test cleanup
				}(conn)
			}
		}()

		// 临时缩短超时以加速测试（生产值 5s，测试只验证"有界"特性）
		t.Cleanup(func() { detectTimeout = DefaultDetectTimeout })
		detectTimeout = 200 * time.Millisecond

		// 故意把 go-redis 的 DialTimeout/ReadTimeout 拉很长，证明真正起作用的是我们 200ms 的 ctx 超时
		client := redis.NewClient(&redis.Options{
			Addr:        ln.Addr().String(),
			DialTimeout: 30 * time.Second,
			ReadTimeout: 30 * time.Second,
		})
		defer client.Close()

		start := time.Now()
		mode, err := DetectScriptModeBounded(client)
		elapsed := time.Since(start)

		require.Error(t, err, "黑洞场景应返回 ctx 超时错误")
		assert.Equal(t, ScriptModeLua, mode, "网络错误时回退 Lua 是文档化的安全默认")
		assert.Less(t, elapsed, 2*time.Second,
			"应在 detectTimeout(200ms) 内返回，绝不能等到 ReadTimeout(30s) 才解阻塞")
	})
}

// ============================================================================
// IsScriptUnsupportedError 测试
// ============================================================================

func TestIsScriptUnsupportedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"普通错误", errors.New("some error"), false},
		{"网络错误", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, false},
		{"unknown command", errors.New("ERR unknown command 'eval'"), true},
		{"auth permission deny", errors.New("ERR auth permission deny"), true},
		{"NOSCRIPT", errors.New("NOSCRIPT No matching script"), true},
		{"cluster support disabled", errors.New("ERR This instance has cluster support disabled"), true},
		{"not allowed", errors.New("ERR command 'EVAL' not allowed"), true},
		{"NOPERM ACL", errors.New("NOPERM this user has no permissions to run the 'eval' command"), true},
		{"NOPERM ACL v7", errors.New("NOPERM User default has no permissions to run the 'eval' command"), true},
		{"OOM 非脚本错误", errors.New("OOM command not allowed when used memory > 'maxmemory'"), false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsScriptUnsupportedError(tt.err))
		})
	}
}
