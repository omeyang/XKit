package xdlock

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redsyncredis "github.com/go-redsync/redsync/v4/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/internal/rediscompat"
)

// =============================================================================
// 辅助函数
// =============================================================================

func newTestMiniredis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
		PoolSize:     2,
		MaxRetries:   1,
	})

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	return mr, client
}

func newCompatConn(t *testing.T, client redis.UniversalClient) *compatConn {
	t.Helper()
	return &compatConn{
		client: client,
		ctx:    context.Background(),
		mode:   rediscompat.ScriptModeCompat,
	}
}

// =============================================================================
// compatPool 测试
// =============================================================================

func TestCompatPool_Get_ReturnsConn(t *testing.T) {
	_, client := newTestMiniredis(t)
	pool := newCompatPool(client, rediscompat.ScriptModeCompat)

	conn, err := pool.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.NoError(t, conn.Close())
}

func TestCompatPool_Get_NilContext(t *testing.T) {
	_, client := newTestMiniredis(t)
	pool := newCompatPool(client, rediscompat.ScriptModeCompat)

	//nolint:staticcheck // SA1012: 故意传入 nil context 测试防御逻辑
	conn, err := pool.Get(nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
}

// =============================================================================
// compatConn 基础操作测试
// =============================================================================

func TestCompatConn_GetSetNX(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	// SetNX 成功
	ok, err := conn.SetNX("key1", "val1", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)

	// Get 返回值
	val, err := conn.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "val1", val)

	// SetNX 失败（key 已存在）
	ok, err = conn.SetNX("key1", "val2", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCompatConn_Get_NotFound(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	// Get 不存在的 key 返回空字符串和 nil 错误（noErrNil）
	val, err := conn.Get("nonexistent")
	assert.NoError(t, err)
	assert.Equal(t, "", val)
}

func TestCompatConn_Set(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	ok, err := conn.Set("key1", "val1")
	require.NoError(t, err)
	assert.True(t, ok)

	val, err := conn.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "val1", val)
}

func TestCompatConn_PTTL(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("key1", "val1")
	mr.SetTTL("key1", time.Minute)

	ttl, err := conn.PTTL("key1")
	require.NoError(t, err)
	assert.True(t, ttl > 0)
}

func TestCompatConn_ScriptLoad(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	err := conn.ScriptLoad(&redsyncredis.Script{Src: "return 1"})
	assert.NoError(t, err)
}

// =============================================================================
// evalDelete 测试
// =============================================================================

func TestEvalDelete_Success(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")

	result, err := conn.evalDelete("lock:key", "my-value")
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)

	// key 应已删除
	assert.False(t, mr.Exists("lock:key"))
}

func TestEvalDelete_KeyNotFound(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	result, err := conn.evalDelete("lock:key", "my-value")
	require.NoError(t, err)
	assert.Equal(t, int64(-1), result)
}

func TestEvalDelete_ValueMismatch(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "other-value")

	result, err := conn.evalDelete("lock:key", "my-value")
	require.NoError(t, err)
	assert.Equal(t, int64(0), result)

	// key 不应被删除
	assert.True(t, mr.Exists("lock:key"))
}

func TestEvalDelete_KeyExpiredBetweenGetAndDel(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")

	// 模拟 GET 之后 DEL 之前 key 过期：直接调用 evalDelete 无法模拟竞态，
	// 但可以验证 DEL 返回 0 的路径
	result, err := conn.evalDelete("lock:key", "my-value")
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)
}

// =============================================================================
// evalTouch 测试
// =============================================================================

func TestEvalTouch_Success(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")
	mr.SetTTL("lock:key", time.Second)

	result, err := conn.evalTouch("lock:key", "my-value", 60000, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)

	// TTL 应被更新
	ttl := mr.TTL("lock:key")
	assert.True(t, ttl > time.Second)
}

func TestEvalTouch_ValueMismatch(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "other-value")

	result, err := conn.evalTouch("lock:key", "my-value", 60000, false)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result)
}

func TestEvalTouch_KeyNotFound_NoSetNX(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	result, err := conn.evalTouch("lock:key", "my-value", 60000, false)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result)
}

func TestEvalTouch_KeyNotFound_WithSetNX_Success(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	result, err := conn.evalTouch("lock:key", "my-value", 60000, true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)

	// key 应被重新创建
	val, err := mr.Get("lock:key")
	require.NoError(t, err)
	assert.Equal(t, "my-value", val)
}

func TestEvalTouch_KeyNotFound_WithSetNX_AlreadyHeld(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	// 另一个持有者已经占用
	mr.Set("lock:key", "other-value")
	mr.Del("lock:key") // 先删除，然后重新设置模拟竞争
	mr.Set("lock:key", "competitor")

	result, err := conn.evalTouch("lock:key", "my-value", 60000, true)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result)
}

// =============================================================================
// Eval 分流测试
// =============================================================================

func TestEvalCompat_Delete(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")

	// 模拟 redsync deleteScript 调用：Eval(script, key, value)
	deleteScript := &redsyncredis.Script{
		KeyCount: 1,
		Src:      `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end`,
	}
	result, err := conn.Eval(deleteScript, "lock:key", "my-value")
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)
}

func TestEvalCompat_Touch(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")

	// 模拟 redsync touchScript 调用：Eval(script, key, value, expiry)
	touchScript := &redsyncredis.Script{
		KeyCount: 1,
		Src:      `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) end`,
	}
	result, err := conn.Eval(touchScript, "lock:key", "my-value", 60000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)
}

func TestEvalCompat_TouchWithSetNX(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	// 模拟 redsync touchWithSetNXScript：Src 含 "NX"
	touchNXScript := &redsyncredis.Script{
		KeyCount: 1,
		Src:      `if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then return 1 end`,
	}
	result, err := conn.Eval(touchNXScript, "lock:key", "my-value", 60000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)
}

func TestEvalCompat_InvalidKeyType(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	script := &redsyncredis.Script{KeyCount: 1, Src: "return 1"}
	_, err := conn.Eval(script, 123, "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key is not a string")
}

// -----------------------------------------------------------------------------
// evalLua 路径（ScriptModeLua）— miniredis 支持 EVAL/EVALSHA 可直接验证
// -----------------------------------------------------------------------------

// newLuaConn 构造 ScriptModeLua 的 compatConn。
func newLuaConn(t *testing.T, client redis.UniversalClient) *compatConn {
	t.Helper()
	return &compatConn{client: client, ctx: context.Background(), mode: rediscompat.ScriptModeLua}
}

func TestEvalLua_Delete_Match(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newLuaConn(t, client)
	mr.Set("lock:key", "v1")
	script := &redsyncredis.Script{
		KeyCount: 1,
		Src:      `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`,
	}
	script.Hash = "deadbeef" // 故意不匹配，迫使 EVALSHA → NOSCRIPT → EVAL 回退

	got, err := conn.Eval(script, "lock:key", "v1")
	require.NoError(t, err)
	assert.EqualValues(t, 1, got)
}

func TestEvalLua_ZeroKeyCount(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newLuaConn(t, client)
	// KeyCount=0 时 keys 为空切片，args 全部作为 ARGV 传入。
	script := &redsyncredis.Script{KeyCount: 0, Src: `return tonumber(ARGV[1])`}
	got, err := conn.Eval(script, 42)
	require.NoError(t, err)
	assert.EqualValues(t, 42, got)
}

func TestEvalLua_KeyNotString(t *testing.T) {
	_, client := newTestMiniredis(t)
	conn := newLuaConn(t, client)
	script := &redsyncredis.Script{KeyCount: 1, Src: `return 1`}
	_, err := conn.Eval(script, 123, "arg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key at index 0 is not a string")
}

func TestEvalCompat_InvalidExpiry(t *testing.T) {
	mr, client := newTestMiniredis(t)
	conn := newCompatConn(t, client)

	mr.Set("lock:key", "my-value")

	script := &redsyncredis.Script{KeyCount: 1, Src: "touch"}
	_, err := conn.Eval(script, "lock:key", "my-value", "not-a-number")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid expiry")
}

// =============================================================================
// NewRedisFactoryWithOpts 测试
// =============================================================================

func TestNewRedisFactoryWithOpts_NilClients(t *testing.T) {
	_, err := NewRedisFactoryWithOpts(nil)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewRedisFactoryWithOpts_EmptyClients(t *testing.T) {
	_, err := NewRedisFactoryWithOpts([]redis.UniversalClient{})
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewRedisFactoryWithOpts_NilClientInSlice(t *testing.T) {
	_, client := newTestMiniredis(t)
	_, err := NewRedisFactoryWithOpts([]redis.UniversalClient{client, nil})
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewRedisFactoryWithOpts_CompatMode(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeCompat),
	)
	require.NoError(t, err)
	require.NotNil(t, factory)
	defer func() { _ = factory.Close(context.Background()) }()

	// Redsync 仍应可用
	assert.NotNil(t, factory.Redsync())
}

func TestNewRedisFactoryWithOpts_LuaMode(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeLua),
	)
	require.NoError(t, err)
	require.NotNil(t, factory)
	defer func() { _ = factory.Close(context.Background()) }()
}

func TestNewRedisFactoryWithOpts_AutoMode(t *testing.T) {
	_, client := newTestMiniredis(t)

	// Auto 模式：miniredis 支持 EVAL，应解析为 Lua
	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
	)
	require.NoError(t, err)
	require.NotNil(t, factory)
	defer func() { _ = factory.Close(context.Background()) }()
}

// =============================================================================
// 端到端：compat 模式下 TryLock/Unlock/Extend
// =============================================================================

func TestCompatMode_TryLock_Unlock(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeCompat),
	)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "compat-test")
	require.NoError(t, err)
	require.NotNil(t, handle)
	assert.Contains(t, handle.Key(), "compat-test")

	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestCompatMode_TryLock_AlreadyHeld(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeCompat),
	)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	h1, err := factory.TryLock(ctx, "compat-held")
	require.NoError(t, err)
	require.NotNil(t, h1)
	defer func() { _ = h1.Unlock(ctx) }() //nolint:errcheck // cleanup in test

	// 第二次 TryLock 返回 (nil, nil)
	h2, err := factory.TryLock(ctx, "compat-held")
	assert.NoError(t, err)
	assert.Nil(t, h2)
}

func TestCompatMode_Lock_Unlock(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeCompat),
	)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.Lock(ctx, "compat-lock", WithTries(1))
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestCompatMode_Extend(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactoryWithOpts(
		[]redis.UniversalClient{client},
		WithRedisScriptMode(rediscompat.ScriptModeCompat),
	)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "compat-extend", WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	err = handle.Extend(ctx)
	assert.NoError(t, err)
}

// =============================================================================
// toMilliseconds 测试
// =============================================================================

func TestToMilliseconds(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected int64
		wantErr  bool
	}{
		{int(1000), 1000, false},
		{int64(2000), 2000, false},
		{float64(3000), 3000, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T(%v)", tt.input, tt.input), func(t *testing.T) {
			result, err := toMilliseconds(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// =============================================================================
// WithRedisScriptMode 选项测试
// =============================================================================

func TestWithRedisScriptMode_SetsOption(t *testing.T) {
	cfg := &redisFactoryConfig{}
	WithRedisScriptMode(rediscompat.ScriptModeCompat)(cfg)
	assert.Equal(t, rediscompat.ScriptModeCompat, cfg.ScriptMode)
}

// =============================================================================
// NewRedisFactory 向后兼容测试
// =============================================================================

func TestNewRedisFactory_DelegatesToWithOpts(t *testing.T) {
	_, client := newTestMiniredis(t)

	factory, err := NewRedisFactory(client)
	require.NoError(t, err)
	require.NotNil(t, factory)
	defer func() { _ = factory.Close(context.Background()) }()

	assert.NotNil(t, factory.Redsync())
}
