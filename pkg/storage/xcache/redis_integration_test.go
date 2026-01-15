//go:build integration

package xcache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// =============================================================================
// 测试环境设置
// =============================================================================

func setupRedis(t *testing.T) (redis.UniversalClient, func()) {
	t.Helper()

	if addr := os.Getenv("XKIT_REDIS_ADDR"); addr != "" {
		client := redis.NewClient(&redis.Options{Addr: addr})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skipf("无法连接到 Redis %s: %v", addr, err)
		}
		return client, func() { client.Close() }
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "redis:7.2-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("redis container not available: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("redis host failed: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("redis port failed: %v", err)
	}

	addr := fmt.Sprintf("%s:%s", host, port.Port())
	client := redis.NewClient(&redis.Options{Addr: addr})
	return client, func() {
		client.Close()
		_ = container.Terminate(ctx)
	}
}

// =============================================================================
// 基础 Redis 操作测试
// =============================================================================

func TestRedis_BasicOperations_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Set/Get 基本操作", func(t *testing.T) {
		key := "test:basic:string"
		value := "hello world"

		err := cache.Client().Set(ctx, key, value, time.Minute).Err()
		require.NoError(t, err)

		got, err := cache.Client().Get(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, value, got)
	})

	t.Run("SetNX 操作", func(t *testing.T) {
		key := "test:setnx"

		// 第一次设置应成功
		ok, err := cache.Client().SetNX(ctx, key, "value1", time.Minute).Result()
		require.NoError(t, err)
		assert.True(t, ok)

		// 第二次设置应失败（key 已存在）
		ok, err = cache.Client().SetNX(ctx, key, "value2", time.Minute).Result()
		require.NoError(t, err)
		assert.False(t, ok)

		// 验证值未被覆盖
		val, err := cache.Client().Get(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("TTL 过期", func(t *testing.T) {
		key := "test:ttl:expire"

		err := cache.Client().Set(ctx, key, "expiring", 500*time.Millisecond).Err()
		require.NoError(t, err)

		// 验证 key 存在
		exists, err := cache.Client().Exists(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), exists)

		// 等待过期
		time.Sleep(600 * time.Millisecond)

		// 验证 key 已过期
		exists, err = cache.Client().Exists(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("Hash 操作", func(t *testing.T) {
		key := "test:hash"

		// HSet 多个字段
		err := cache.Client().HSet(ctx, key,
			"field1", "value1",
			"field2", "value2",
			"field3", "value3",
		).Err()
		require.NoError(t, err)

		// HGet 单个字段
		val, err := cache.Client().HGet(ctx, key, "field1").Result()
		require.NoError(t, err)
		assert.Equal(t, "value1", val)

		// HGetAll 获取所有字段
		all, err := cache.Client().HGetAll(ctx, key).Result()
		require.NoError(t, err)
		assert.Len(t, all, 3)
		assert.Equal(t, "value1", all["field1"])
		assert.Equal(t, "value2", all["field2"])
		assert.Equal(t, "value3", all["field3"])

		// HLen 获取字段数量
		length, err := cache.Client().HLen(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)
	})

	t.Run("List 操作", func(t *testing.T) {
		key := "test:list"

		// LPush
		err := cache.Client().LPush(ctx, key, "c", "b", "a").Err()
		require.NoError(t, err)

		// LRange 获取所有元素
		vals, err := cache.Client().LRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, vals)

		// LLen
		length, err := cache.Client().LLen(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)

		// RPop
		val, err := cache.Client().RPop(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "c", val)
	})

	t.Run("Set 操作", func(t *testing.T) {
		key := "test:set"

		// SAdd
		added, err := cache.Client().SAdd(ctx, key, "a", "b", "c", "a").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), added) // "a" 重复不计入

		// SMembers
		members, err := cache.Client().SMembers(ctx, key).Result()
		require.NoError(t, err)
		assert.Len(t, members, 3)

		// SIsMember
		isMember, err := cache.Client().SIsMember(ctx, key, "b").Result()
		require.NoError(t, err)
		assert.True(t, isMember)

		isMember, err = cache.Client().SIsMember(ctx, key, "d").Result()
		require.NoError(t, err)
		assert.False(t, isMember)
	})

	t.Run("ZSet 操作", func(t *testing.T) {
		key := "test:zset"

		// ZAdd
		added, err := cache.Client().ZAdd(ctx, key,
			redis.Z{Score: 1, Member: "one"},
			redis.Z{Score: 2, Member: "two"},
			redis.Z{Score: 3, Member: "three"},
		).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), added)

		// ZRange 按分数排序
		vals, err := cache.Client().ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two", "three"}, vals)

		// ZRangeByScore
		vals, err = cache.Client().ZRangeByScore(ctx, key, &redis.ZRangeBy{
			Min: "1",
			Max: "2",
		}).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two"}, vals)

		// ZScore
		score, err := cache.Client().ZScore(ctx, key, "two").Result()
		require.NoError(t, err)
		assert.Equal(t, float64(2), score)
	})
}

// =============================================================================
// 分布式锁测试
// =============================================================================

func TestRedis_Lock_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Lock/Unlock 基本流程", func(t *testing.T) {
		unlock, err := cache.Lock(ctx, "test:lock:basic", 10*time.Second)
		require.NoError(t, err)
		require.NotNil(t, unlock)

		// 验证锁 key 存在
		exists, err := client.Exists(ctx, "lock:test:lock:basic").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), exists)

		// 释放锁
		err = unlock(ctx)
		require.NoError(t, err)

		// 验证锁 key 已删除
		exists, err = client.Exists(ctx, "lock:test:lock:basic").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("Lock 互斥性", func(t *testing.T) {
		// 第一个锁
		unlock1, err := cache.Lock(ctx, "test:lock:mutex", 10*time.Second)
		require.NoError(t, err)
		defer unlock1(ctx)

		// 创建第二个 cache 实例尝试获取同一个锁
		cache2, err := NewRedis(client)
		require.NoError(t, err)
		defer cache2.Close()

		// 第二个锁应该失败
		_, err = cache2.Lock(ctx, "test:lock:mutex", 10*time.Second)
		assert.ErrorIs(t, err, ErrLockFailed)
	})

	t.Run("Lock 带重试", func(t *testing.T) {
		// 创建带重试配置的 cache
		cacheWithRetry, err := NewRedis(client,
			WithLockRetry(50*time.Millisecond, 3),
		)
		require.NoError(t, err)
		defer cacheWithRetry.Close()

		// 先获取锁
		unlock1, err := cache.Lock(ctx, "test:lock:retry", 200*time.Millisecond)
		require.NoError(t, err)

		// 在另一个 goroutine 中稍后释放锁
		go func() {
			time.Sleep(100 * time.Millisecond)
			unlock1(ctx)
		}()

		// 尝试获取锁（应在重试后成功）
		start := time.Now()
		unlock2, err := cacheWithRetry.Lock(ctx, "test:lock:retry", 5*time.Second)
		elapsed := time.Since(start)

		require.NoError(t, err)
		require.NotNil(t, unlock2)
		assert.Greater(t, elapsed, 50*time.Millisecond, "应该经历至少一次重试")
		unlock2(ctx)
	})

	t.Run("Lock TTL 过期", func(t *testing.T) {
		// 使用很短的 TTL
		unlock, err := cache.Lock(ctx, "test:lock:ttl", 500*time.Millisecond)
		require.NoError(t, err)

		// 等待锁过期
		time.Sleep(600 * time.Millisecond)

		// 释放锁应返回 ErrLockExpired
		err = unlock(ctx)
		assert.ErrorIs(t, err, ErrLockExpired)

		// 现在应该能重新获取锁
		unlock2, err := cache.Lock(ctx, "test:lock:ttl", 10*time.Second)
		require.NoError(t, err)
		unlock2(ctx)
	})

	t.Run("自定义锁前缀", func(t *testing.T) {
		cacheWithPrefix, err := NewRedis(client,
			WithLockKeyPrefix("myapp:lock:"),
		)
		require.NoError(t, err)
		defer cacheWithPrefix.Close()

		unlock, err := cacheWithPrefix.Lock(ctx, "test", 10*time.Second)
		require.NoError(t, err)

		// 验证使用了自定义前缀
		exists, err := client.Exists(ctx, "myapp:lock:test").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), exists)

		unlock(ctx)
	})

	t.Run("并发锁争抢", func(t *testing.T) {
		const goroutines = 10
		var successCount atomic.Int64
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, _ := NewRedis(client)
				defer c.Close()

				unlock, err := c.Lock(ctx, "test:lock:concurrent", 5*time.Second)
				if err == nil {
					successCount.Add(1)
					time.Sleep(50 * time.Millisecond)
					unlock(ctx)
				}
			}()
		}

		wg.Wait()
		// 由于没有重试，只有一个 goroutine 应该成功
		assert.Equal(t, int64(1), successCount.Load())
	})
}

// =============================================================================
// Loader 测试
// =============================================================================

func TestLoader_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Load 基本流程", func(t *testing.T) {
		loader := NewLoader(cache)

		loadCount := 0
		loadFn := func(ctx context.Context) ([]byte, error) {
			loadCount++
			return []byte("loaded-value"), nil
		}

		// 第一次加载（未命中缓存）
		val, err := loader.Load(ctx, "loader:basic", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("loaded-value"), val)
		assert.Equal(t, 1, loadCount)

		// 第二次加载（命中缓存）
		val, err = loader.Load(ctx, "loader:basic", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("loaded-value"), val)
		assert.Equal(t, 1, loadCount) // 未再次调用 loadFn
	})

	t.Run("Load TTL 过期后重新加载", func(t *testing.T) {
		loader := NewLoader(cache)

		loadCount := 0
		loadFn := func(ctx context.Context) ([]byte, error) {
			loadCount++
			return []byte(fmt.Sprintf("value-%d", loadCount)), nil
		}

		// 第一次加载
		val, err := loader.Load(ctx, "loader:ttl", loadFn, 500*time.Millisecond)
		require.NoError(t, err)
		assert.Equal(t, []byte("value-1"), val)

		// 等待过期
		time.Sleep(600 * time.Millisecond)

		// 应重新加载
		val, err = loader.Load(ctx, "loader:ttl", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("value-2"), val)
		assert.Equal(t, 2, loadCount)
	})

	t.Run("Singleflight 并发加载", func(t *testing.T) {
		loader := NewLoader(cache, WithSingleflight(true))

		// 清理 key 确保测试干净
		client.Del(ctx, "loader:singleflight")

		var loadCount atomic.Int64
		loadFn := func(ctx context.Context) ([]byte, error) {
			loadCount.Add(1)
			time.Sleep(100 * time.Millisecond) // 模拟慢加载
			return []byte("shared-value"), nil
		}

		const goroutines = 10
		var wg sync.WaitGroup
		results := make([][]byte, goroutines)
		errs := make([]error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx], errs[idx] = loader.Load(ctx, "loader:singleflight", loadFn, time.Minute)
			}(i)
		}

		wg.Wait()

		// 验证所有请求都成功
		for i := 0; i < goroutines; i++ {
			require.NoError(t, errs[i])
			assert.Equal(t, []byte("shared-value"), results[i])
		}

		// 由于 singleflight，loadFn 应该只被调用一次
		assert.Equal(t, int64(1), loadCount.Load(), "singleflight 应确保只加载一次")
	})

	t.Run("Load 加载错误处理", func(t *testing.T) {
		loader := NewLoader(cache)

		loadErr := errors.New("load failed")
		loadFn := func(ctx context.Context) ([]byte, error) {
			return nil, loadErr
		}

		// 应该返回加载错误
		_, err := loader.Load(ctx, "loader:error", loadFn, time.Minute)
		assert.ErrorIs(t, err, loadErr)

		// 缓存中不应该有值
		exists, err := client.Exists(ctx, "loader:error").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("LoadHash 基本流程", func(t *testing.T) {
		loader := NewLoader(cache)

		loadCount := 0
		loadFn := func(ctx context.Context) ([]byte, error) {
			loadCount++
			return []byte("hash-value"), nil
		}

		// 第一次加载
		val, err := loader.LoadHash(ctx, "loader:hash", "field1", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("hash-value"), val)
		assert.Equal(t, 1, loadCount)

		// 第二次加载（命中缓存）
		val, err = loader.LoadHash(ctx, "loader:hash", "field1", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("hash-value"), val)
		assert.Equal(t, 1, loadCount)

		// 不同 field 应重新加载
		val, err = loader.LoadHash(ctx, "loader:hash", "field2", loadFn, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("hash-value"), val)
		assert.Equal(t, 2, loadCount)
	})

	t.Run("分布式锁保护加载", func(t *testing.T) {
		loader := NewLoader(cache,
			WithDistributedLock(true),
			WithDistributedLockTTL(5*time.Second),
		)

		// 清理 key
		client.Del(ctx, "loader:distlock")

		var loadCount atomic.Int64
		loadFn := func(ctx context.Context) ([]byte, error) {
			loadCount.Add(1)
			time.Sleep(100 * time.Millisecond)
			return []byte("protected-value"), nil
		}

		const goroutines = 5
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := loader.Load(ctx, "loader:distlock", loadFn, time.Minute)
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// 分布式锁应确保只加载一次
		assert.Equal(t, int64(1), loadCount.Load(), "分布式锁应确保只加载一次")
	})
}

// =============================================================================
// 大数据量测试
// =============================================================================

func TestRedis_LargeData_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("大字符串存储", func(t *testing.T) {
		// 1MB 数据
		largeValue := make([]byte, 1<<20)
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		err := cache.Client().Set(ctx, "test:large:string", largeValue, time.Minute).Err()
		require.NoError(t, err)

		got, err := cache.Client().Get(ctx, "test:large:string").Bytes()
		require.NoError(t, err)
		assert.Equal(t, largeValue, got)
	})

	t.Run("批量操作", func(t *testing.T) {
		// Pipeline 批量设置
		pipe := cache.Client().Pipeline()
		for i := 0; i < 1000; i++ {
			pipe.Set(ctx, fmt.Sprintf("test:batch:%d", i), fmt.Sprintf("value-%d", i), time.Minute)
		}
		_, err := pipe.Exec(ctx)
		require.NoError(t, err)

		// Pipeline 批量获取
		pipe = cache.Client().Pipeline()
		for i := 0; i < 1000; i++ {
			pipe.Get(ctx, fmt.Sprintf("test:batch:%d", i))
		}
		cmds, err := pipe.Exec(ctx)
		require.NoError(t, err)

		for i, cmd := range cmds {
			val, err := cmd.(*redis.StringCmd).Result()
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("value-%d", i), val)
		}
	})

	t.Run("大 Hash", func(t *testing.T) {
		key := "test:large:hash"

		// 设置 1000 个字段
		fields := make([]any, 2000)
		for i := 0; i < 1000; i++ {
			fields[i*2] = fmt.Sprintf("field-%d", i)
			fields[i*2+1] = fmt.Sprintf("value-%d", i)
		}

		err := cache.Client().HSet(ctx, key, fields...).Err()
		require.NoError(t, err)

		// 验证字段数量
		length, err := cache.Client().HLen(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1000), length)
	})
}

// =============================================================================
// 错误处理测试
// =============================================================================

func TestRedis_ErrorHandling_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("获取不存在的 key", func(t *testing.T) {
		_, err := cache.Client().Get(ctx, "nonexistent:key").Result()
		assert.ErrorIs(t, err, redis.Nil)
	})

	t.Run("类型错误", func(t *testing.T) {
		// 设置 string 类型
		err := cache.Client().Set(ctx, "test:type:error", "string-value", time.Minute).Err()
		require.NoError(t, err)

		// 尝试当作 list 操作
		_, err = cache.Client().LRange(ctx, "test:type:error", 0, -1).Result()
		assert.Error(t, err) // WRONGTYPE 错误
	})

	t.Run("Context 取消", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // 立即取消

		err := cache.Client().Set(cancelCtx, "test:cancel", "value", time.Minute).Err()
		assert.Error(t, err)
	})

	t.Run("Lock TTL 非法值", func(t *testing.T) {
		_, err := cache.Lock(ctx, "test:invalid:ttl", 0)
		assert.ErrorIs(t, err, ErrInvalidLockTTL)

		_, err = cache.Lock(ctx, "test:invalid:ttl", -1*time.Second)
		assert.ErrorIs(t, err, ErrInvalidLockTTL)
	})
}
