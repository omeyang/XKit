package rediscompat

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ScriptMode 表示 Redis 脚本执行模式。
type ScriptMode int

const (
	// ScriptModeAuto 自动检测（默认）。
	// 在构造函数中执行 EVAL "return 1" 0 探测一次，缓存结果。
	ScriptModeAuto ScriptMode = iota

	// ScriptModeLua 强制使用 Lua 脚本。
	// 跳过探测，直接使用 EVAL/EVALSHA 执行脚本。
	ScriptModeLua

	// ScriptModeCompat 兼容模式（基础命令）。
	// 跳过探测，使用 Pipeline 基础命令替代 Lua 脚本。
	ScriptModeCompat
)

// String 返回脚本模式的字符串表示。
func (m ScriptMode) String() string {
	switch m {
	case ScriptModeAuto:
		return "auto"
	case ScriptModeLua:
		return "lua"
	case ScriptModeCompat:
		return "compat"
	default:
		return "unknown"
	}
}

// IsValid 检查脚本模式是否有效。
func (m ScriptMode) IsValid() bool {
	switch m {
	case ScriptModeAuto, ScriptModeLua, ScriptModeCompat:
		return true
	default:
		return false
	}
}

// DetectScriptMode 通过执行 EVAL "return 1" 0 探测 Redis 是否支持 Lua 脚本。
//
// 返回值：
//   - ScriptModeLua: Redis 支持 Lua 脚本（EVAL 成功）
//   - ScriptModeCompat: Redis 不支持 Lua 脚本（代理返回权限/不支持错误）
//   - (ScriptModeLua, err): 网络错误等非脚本相关错误，返回错误让调用方决定
//
// 设计决策: 网络错误时返回 ScriptModeLua 而非 ScriptModeCompat，
// 因为网络错误不代表不支持脚本，应让调用方根据场景决定处理方式。
func DetectScriptMode(ctx context.Context, client redis.UniversalClient) (ScriptMode, error) {
	err := client.Eval(ctx, "return 1", nil).Err()
	if err == nil {
		return ScriptModeLua, nil
	}

	if IsScriptUnsupportedError(err) {
		return ScriptModeCompat, nil
	}

	// 网络错误等非脚本相关错误
	return ScriptModeLua, err
}

// scriptUnsupportedPatterns 代理返回的脚本不支持错误的匹配模式
var scriptUnsupportedPatterns = []string{
	"unknown command",          // Twemproxy/Codis: "ERR unknown command 'eval'"
	"auth permission deny",     // Predixy: "ERR auth permission deny"
	"NOSCRIPT",                 // EVAL 也失败时
	"cluster support disabled", // 集群支持被禁用
	"not allowed",              // 通用权限拒绝
}

// IsScriptUnsupportedError 检查错误是否表示 Redis 不支持 Lua 脚本。
//
// 匹配以下代理错误模式：
//   - "unknown command": Twemproxy/Codis 不支持 EVAL
//   - "auth permission deny": Predixy 权限拒绝
//   - "NOSCRIPT": EVAL 也失败时
//   - "cluster support disabled": 集群支持被禁用
//   - "not allowed": 通用权限拒绝
func IsScriptUnsupportedError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	for _, pattern := range scriptUnsupportedPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
