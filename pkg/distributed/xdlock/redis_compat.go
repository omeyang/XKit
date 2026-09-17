package xdlock

import (
	"context"
	"fmt"
	"strings"
	"time"

	redsyncredis "github.com/go-redsync/redsync/v4/redis"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/internal/rediscompat"
)

// =============================================================================
// compatPool — 替换 goredis.NewPool，在 compat 模式下拦截 Eval
// =============================================================================

// compatPool 实现 redsyncredis.Pool，在 compat 模式下将 Lua 脚本翻译为基础命令。
type compatPool struct {
	client redis.UniversalClient
	mode   rediscompat.ScriptMode
}

// newCompatPool 创建 compatPool。
func newCompatPool(client redis.UniversalClient, mode rediscompat.ScriptMode) redsyncredis.Pool {
	return &compatPool{client: client, mode: mode}
}

func (p *compatPool) Get(ctx context.Context) (redsyncredis.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return &compatConn{client: p.client, ctx: ctx, mode: p.mode}, nil
}

// =============================================================================
// compatConn — 实现 redsyncredis.Conn，拦截 Eval 调用
// =============================================================================

// compatConn 实现 redsyncredis.Conn。
// Get/Set/SetNX/PTTL/Close 直接委托给 client（同 goredis 适配器）。
// Eval 在 compat 模式下翻译为基础命令。
type compatConn struct {
	client redis.UniversalClient
	ctx    context.Context
	mode   rediscompat.ScriptMode
}

func (c *compatConn) Get(name string) (string, error) {
	value, err := c.client.Get(c.ctx, name).Result()
	return value, noErrNil(err)
}

func (c *compatConn) Set(name string, value string) (bool, error) {
	reply, err := c.client.Set(c.ctx, name, value, 0).Result()
	return reply == "OK", err
}

func (c *compatConn) SetNX(name string, value string, expiry time.Duration) (bool, error) {
	return c.client.SetNX(c.ctx, name, value, expiry).Result()
}

func (c *compatConn) PTTL(name string) (time.Duration, error) {
	return c.client.PTTL(c.ctx, name).Result()
}

func (c *compatConn) Close() error {
	return nil
}

func (c *compatConn) ScriptLoad(_ *redsyncredis.Script) error {
	// compat 模式不需要预加载脚本
	return nil
}

// Eval 拦截 redsync 的 Lua 脚本调用。
// compat 模式下根据参数数量翻译为基础命令；lua 模式下走标准 EvalSha → Eval 回退。
//
// redsync 脚本清单（KeyCount 均为 1）：
//   - deleteScript: Eval(script, key, value)          → 2 个 keysAndArgs
//   - touchScript:  Eval(script, key, value, expiry)  → 3 个 keysAndArgs
//   - touchWithSetNXScript: 同 touchScript，脚本 Src 含 "NX"
func (c *compatConn) Eval(script *redsyncredis.Script, keysAndArgs ...interface{}) (interface{}, error) {
	if c.mode == rediscompat.ScriptModeCompat {
		return c.evalCompat(script, keysAndArgs...)
	}
	return c.evalLua(script, keysAndArgs...)
}

// evalLua 标准 EvalSha → Eval 回退（同 goredis 适配器逻辑）。
func (c *compatConn) evalLua(script *redsyncredis.Script, keysAndArgs ...interface{}) (interface{}, error) {
	keys := make([]string, script.KeyCount)
	args := keysAndArgs

	if script.KeyCount > 0 {
		for i := range script.KeyCount {
			k, ok := keysAndArgs[i].(string)
			if !ok {
				return nil, fmt.Errorf("xdlock: eval: key at index %d is not a string", i)
			}
			keys[i] = k
		}
		args = keysAndArgs[script.KeyCount:]
	}

	v, err := c.client.EvalSha(c.ctx, script.Hash, keys, args...).Result()
	if err != nil && strings.HasPrefix(err.Error(), "NOSCRIPT ") {
		v, err = c.client.Eval(c.ctx, script.Src, keys, args...).Result()
	}
	return v, noErrNil(err)
}

// evalCompat 将 redsync Lua 脚本翻译为基础命令。
//
// 识别策略：
//   - keysAndArgs 数量 == 2（key + value）→ deleteScript
//   - keysAndArgs 数量 == 3（key + value + expiry）→ touchScript 或 touchWithSetNXScript
//   - 其他 → 回退到 evalLua（防御性）
func (c *compatConn) evalCompat(script *redsyncredis.Script, keysAndArgs ...interface{}) (interface{}, error) {
	args := keysAndArgs[script.KeyCount:]
	key, ok := keysAndArgs[0].(string)
	if !ok {
		return nil, fmt.Errorf("xdlock: compat eval: key is not a string")
	}

	switch len(args) {
	case 1:
		// deleteScript: args = [value]
		return c.evalDelete(key, fmt.Sprint(args[0]))
	case 2:
		// touchScript 或 touchWithSetNXScript: args = [value, expiry]
		value := fmt.Sprint(args[0])
		expiry, err := toMilliseconds(args[1])
		if err != nil {
			return nil, fmt.Errorf("xdlock: compat eval: invalid expiry: %w", err)
		}
		setNX := strings.Contains(script.Src, "NX")
		return c.evalTouch(key, value, expiry, setNX)
	default:
		// 未知脚本，回退到 Lua 执行
		return c.evalLua(script, keysAndArgs...)
	}
}

// evalDelete 翻译 deleteScript: GET key → if match → DEL key。
//
// 返回值语义与 redsync deleteScript 一致：
//   - int64(1): 成功删除
//   - int64(0): value 不匹配（锁被抢走）
//   - int64(-1): key 不存在（锁已过期）
//
// 竞态分析：GET-DEL 之间锁可能过期被重获取，DEL 删了新持有者的锁。
// 窗口微秒级，redsync 的 Redlock 语义本身允许此级别误差。
func (c *compatConn) evalDelete(key, value string) (interface{}, error) {
	val, err := c.client.Get(c.ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return int64(-1), nil
		}
		return nil, err
	}
	if val != value {
		return int64(0), nil
	}

	deleted, err := c.client.Del(c.ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if deleted == 0 {
		return int64(-1), nil
	}
	return int64(1), nil
}

// evalTouch 翻译 touchScript / touchWithSetNXScript:
// GET key → if match → PEXPIRE key expiry [→ if nil + setNX → SET key value NX PX expiry]。
//
// 返回值语义与 redsync touchScript 一致：
//   - int64(1): 成功续期（或 SetNX 成功）
//   - int64(0): value 不匹配且 SetNX 未启用/失败
func (c *compatConn) evalTouch(key, value string, expiryMs int64, setNX bool) (interface{}, error) {
	expiry := time.Duration(expiryMs) * time.Millisecond

	val, err := c.client.Get(c.ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if err == nil && val == value {
		return c.touchPExpire(key, expiry)
	}

	// key 不存在或 value 不匹配
	if setNX && err == redis.Nil {
		return c.touchSetNX(key, value, expiry)
	}

	return int64(0), nil
}

// touchPExpire 续期已匹配的锁。
func (c *compatConn) touchPExpire(key string, expiry time.Duration) (interface{}, error) {
	ok, err := c.client.PExpire(c.ctx, key, expiry).Result()
	if err != nil {
		return nil, err
	}
	if ok {
		return int64(1), nil
	}
	return int64(0), nil
}

// touchSetNX 尝试重新获取已过期的锁（touchWithSetNXScript）。
func (c *compatConn) touchSetNX(key, value string, expiry time.Duration) (interface{}, error) {
	ok, err := c.client.SetNX(c.ctx, key, value, expiry).Result()
	if err != nil {
		return nil, err
	}
	if ok {
		return int64(1), nil
	}
	return int64(0), nil
}

// toMilliseconds 将 interface{} 转为毫秒数。
// redsync 传入的 expiry 是 int（毫秒数）。
func toMilliseconds(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int:
		return int64(val), nil
	case int64:
		return val, nil
	case float64:
		return int64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

// noErrNil 将 redis.Nil 转为 nil（同 goredis 适配器）。
func noErrNil(err error) error {
	if err == redis.Nil {
		return nil
	}
	return err
}
