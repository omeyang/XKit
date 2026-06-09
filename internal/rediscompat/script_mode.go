package rediscompat

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultDetectTimeout 是 DetectScriptModeBounded 默认使用的超时。
//
// 探测命令是单次 EVAL "return 1" 0（0 个 key），正常 Redis 路径下 RTT 通常 < 10ms；
// 5 秒上限只用于在 Redis 地址被防火墙/路由黑洞或客户端未配置 DialTimeout 时，
// 防止构造函数无限阻塞。需要更短/更长边界的调用方应自带 context.WithTimeout。
const DefaultDetectTimeout = 5 * time.Second

// detectTimeout 是 DetectScriptModeBounded 实际使用的超时，作为包内可变变量
// 仅供测试通过 t.Cleanup 临时缩短以加速超时验证；生产路径只读 DefaultDetectTimeout 同值。
var detectTimeout = DefaultDetectTimeout

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
//
// 注意: 本函数完全遵守传入的 ctx——没有 deadline 的 ctx 在 Redis 黑洞场景下
// 可能无限阻塞。构造函数等无 caller-ctx 路径请改用 [DetectScriptModeBounded]。
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

// DetectScriptModeBounded 是 [DetectScriptMode] 的有界超时版本，专供构造函数等
// 无 caller-ctx 的场景使用：内部 [DefaultDetectTimeout] 超时硬封顶，防止 Redis 地址
// 黑洞 / 客户端把 DialTimeout/ReadTimeout 配成 0 时构造函数无限阻塞。
//
// 实现选择 goroutine+select 而非单纯传 ctx：go-redis v9 默认 ContextTimeoutEnabled=false，
// `client.Eval(ctx, ...)` 不会因 ctx 超时主动结束底层 socket read——必须靠独立 goroutine
// 跑探测、调用方在 ctx 触发时直接返回，才能保证硬上限。代价：超时时内部 goroutine 仍在跑，
// 直到 go-redis 自身的 Dial/Read 超时（默认 5s/3s）才会回收；调用方把这些都配成 0
// 同时 Redis 真是黑洞时，goroutine 会泄漏到进程结束——这是 caller 配置选择的代价，
// 与"构造函数永远不返回"二选一时显然前者可接受。
//
// 返回语义与 [DetectScriptMode] 一致：超时/网络错误时返回 (ScriptModeLua, err)，
// 让调用方按需降级。
func DetectScriptModeBounded(client redis.UniversalClient) (ScriptMode, error) {
	type detectResult struct {
		mode ScriptMode
		err  error
	}
	// buffered=1 保证内部 goroutine 哪怕在超时之后才完成也能写入 + 退出，不会因无人接收而阻塞。
	resCh := make(chan detectResult, 1)
	go func() {
		mode, err := DetectScriptMode(context.Background(), client)
		resCh <- detectResult{mode, err}
	}()

	timer := time.NewTimer(detectTimeout)
	defer timer.Stop()
	select {
	case res := <-resCh:
		return res.mode, res.err
	case <-timer.C:
		return ScriptModeLua, context.DeadlineExceeded
	}
}

// scriptUnsupportedPatterns 代理返回的脚本不支持错误的匹配模式
var scriptUnsupportedPatterns = []string{
	"unknown command",          // Twemproxy/Codis: "ERR unknown command 'eval'"
	"auth permission deny",     // Predixy: "ERR auth permission deny"
	"NOSCRIPT",                 // EVAL 也失败时
	"cluster support disabled", // 集群支持被禁用
	"not allowed",              // 通用权限拒绝（ERR command 'EVAL' not allowed）
	"NOPERM",                   // Redis 6+ ACL: "NOPERM ... 'eval' ..."
}

// IsScriptUnsupportedError 检查错误是否表示 Redis 不支持 Lua 脚本。
//
// 匹配以下代理错误模式：
//   - "unknown command": Twemproxy/Codis 不支持 EVAL
//   - "auth permission deny": Predixy 权限拒绝
//   - "NOSCRIPT": EVAL 也失败时
//   - "cluster support disabled": 集群支持被禁用
//   - "not allowed": 通用权限拒绝
//   - "NOPERM": Redis 6+ ACL 权限拒绝
//
// 排除 OOM 等瞬态服务端错误——OOM 不代表永久不支持脚本。
func IsScriptUnsupportedError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// OOM 是瞬态条件（"OOM command not allowed when used memory > 'maxmemory'"），
	// 含 "not allowed" 子串但不代表 Redis 不支持脚本，须排除。
	if strings.HasPrefix(errStr, "OOM") {
		return false
	}

	for _, pattern := range scriptUnsupportedPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
