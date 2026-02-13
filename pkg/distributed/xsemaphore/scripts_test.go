package xsemaphore

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetScripts(t *testing.T) {
	scripts := getScripts()
	require.NotNil(t, scripts)

	// 验证所有脚本都已初始化
	assert.NotNil(t, scripts.acquire)
	assert.NotNil(t, scripts.release)
	assert.NotNil(t, scripts.extend)
	assert.NotNil(t, scripts.query)

	// 多次调用应返回同一实例（单例模式）
	scripts2 := getScripts()
	assert.Same(t, scripts, scripts2)
}

func TestWarmupScripts(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx := context.Background()

	t.Run("nil client returns error", func(t *testing.T) {
		err := WarmupScripts(ctx, nil)
		assert.ErrorIs(t, err, ErrNilClient)
	})

	t.Run("successful warmup", func(t *testing.T) {
		err := WarmupScripts(ctx, client)
		assert.NoError(t, err)
	})

	t.Run("multiple warmups succeed", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			err := WarmupScripts(ctx, client)
			assert.NoError(t, err)
		}
	})
}

func TestLuaScripts_Embedded(t *testing.T) {
	// 验证 Lua 脚本已正确嵌入
	assert.NotEmpty(t, acquireLuaSource)
	assert.NotEmpty(t, releaseLuaSource)
	assert.NotEmpty(t, extendLuaSource)
	assert.NotEmpty(t, queryLuaSource)

	// 验证脚本包含预期的内容
	assert.Contains(t, acquireLuaSource, "ZREMRANGEBYSCORE")
	assert.Contains(t, acquireLuaSource, "ZADD")
	assert.Contains(t, releaseLuaSource, "ZREM")
	assert.Contains(t, extendLuaSource, "ZSCORE")
	assert.Contains(t, queryLuaSource, "ZCOUNT")
}

func TestScripts_Execute(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	ctx := context.Background()
	scripts := getScripts()

	t.Run("acquire script", func(t *testing.T) {
		result, err := scripts.acquire.Run(ctx, client,
			[]string{"test:permits", "test:tenant"},
			0,       // now
			1000000, // expiresAt
			"permit-1",
			10,    // capacity
			5,     // tenantQuota
			60000, // keyTTLMargin
		).Int64Slice()
		require.NoError(t, err)
		assert.Equal(t, int64(0), result[0]) // success
	})

	t.Run("release script", func(t *testing.T) {
		result, err := scripts.release.Run(ctx, client,
			[]string{"test:permits", "test:tenant"},
			"permit-1",
		).Int64Slice()
		require.NoError(t, err)
		assert.Equal(t, int64(0), result[0]) // success
	})

	t.Run("query script", func(t *testing.T) {
		result, err := scripts.query.Run(ctx, client,
			[]string{"test:permits", "test:tenant"},
			0, // now
		).Int64Slice()
		require.NoError(t, err)
		assert.Equal(t, int64(0), result[0]) // global count
		assert.Equal(t, int64(0), result[1]) // tenant count
	})
}
