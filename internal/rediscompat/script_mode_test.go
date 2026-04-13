package rediscompat

import (
	"context"
	"errors"
	"net"
	"testing"

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
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsScriptUnsupportedError(tt.err))
		})
	}
}
