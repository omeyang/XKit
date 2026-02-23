package xcron

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopLocker(t *testing.T) {
	locker := NoopLocker()
	require.NotNil(t, locker)

	ctx := context.Background()

	t.Run("TryLock always returns handle", func(t *testing.T) {
		handle, err := locker.TryLock(ctx, "key1", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle)

		// 同一个 key 再次获取也返回 handle（NoopLocker 不做互斥）
		handle2, err := locker.TryLock(ctx, "key1", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle2)
	})

	t.Run("Unlock always succeeds", func(t *testing.T) {
		handle, _ := locker.TryLock(ctx, "key1", time.Minute) //nolint:errcheck // 测试代码，NoopLocker 总是成功
		err := handle.Unlock(ctx)
		assert.NoError(t, err)

		// 多次 Unlock 也成功（NoopLocker 的特性）
		err = handle.Unlock(ctx)
		assert.NoError(t, err)
	})

	t.Run("Renew always succeeds", func(t *testing.T) {
		handle, _ := locker.TryLock(ctx, "key1", time.Minute) //nolint:errcheck // 测试代码，NoopLocker 总是成功
		err := handle.Renew(ctx, time.Minute)
		assert.NoError(t, err)
	})
}

func TestMockLocker(t *testing.T) {
	locker := newMockLocker()
	ctx := context.Background()

	var handle1 LockHandle

	t.Run("TryLock succeeds on first attempt", func(t *testing.T) {
		var err error
		handle1, err = locker.TryLock(ctx, "key1", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle1)
	})

	t.Run("TryLock fails when already held", func(t *testing.T) {
		// key1 已被持有
		handle, err := locker.TryLock(ctx, "key1", time.Minute)
		assert.NoError(t, err)
		assert.Nil(t, handle)
	})

	t.Run("TryLock succeeds after unlock", func(t *testing.T) {
		err := handle1.Unlock(ctx)
		assert.NoError(t, err)

		handle, err := locker.TryLock(ctx, "key1", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle)
	})

	t.Run("different keys are independent", func(t *testing.T) {
		handle2, err := locker.TryLock(ctx, "key2", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle2)

		handle3, err := locker.TryLock(ctx, "key3", time.Minute)
		assert.NoError(t, err)
		assert.NotNil(t, handle3)
	})
}

func TestNoopLockHandle_Key(t *testing.T) {
	locker := NoopLocker()
	handle, err := locker.TryLock(context.Background(), "my-key", time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "my-key", handle.Key())
}

func TestRedisLockHandle_Key(t *testing.T) {
	h := &redisLockHandle{key: "xcron:lock:test-job"}
	assert.Equal(t, "xcron:lock:test-job", h.Key())
}

func TestK8sLockHandle_Key(t *testing.T) {
	h := &k8sLockHandle{key: "test-job"}
	assert.Equal(t, "test-job", h.Key())
}

func TestSanitizeK8sName(t *testing.T) {
	t.Run("clean names unchanged", func(t *testing.T) {
		// 已合法的名称（小写字母、数字、'-'）不添加 hash
		tests := []struct {
			input    string
			expected string
		}{
			{"simple", "simple"},
			{"with-dash", "with-dash"},
			{"", ""},
			{"a", "a"},
			{"a-b-c", "a-b-c"},
			{"123", "123"},
			{"a1b2c3", "a1b2c3"},
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				result := sanitizeK8sName(tt.input, 0)
				assert.Equal(t, tt.expected, result)
				assert.LessOrEqual(t, len(result), 63)
			})
		}
	})

	t.Run("sanitized names get hash suffix", func(t *testing.T) {
		// 需要字符替换/折叠/修剪的名称添加 hash 后缀确保唯一性
		inputs := []string{
			"WITH_UPPER",
			"with.dot",
			"with/slash",
			"with:colon",
			"with space",
			"multiple---dashes",
			"-leading-dash",
			"trailing-dash-",
		}

		for _, input := range inputs {
			t.Run(input, func(t *testing.T) {
				result := sanitizeK8sName(input, 0)
				assert.LessOrEqual(t, len(result), 63)
				// 结果应包含 hash 后缀（8 个十六进制字符）
				assert.Regexp(t, `^[a-z0-9-]+-[a-f0-9]{8}$`, result)
			})
		}
	})

	t.Run("long names get hash suffix", func(t *testing.T) {
		longName := "this-is-a-very-long-name-that-exceeds-the-maximum-length-allowed-by-kubernetes-for-resource-names"
		result := sanitizeK8sName(longName, 0)
		assert.LessOrEqual(t, len(result), 63)
		assert.Regexp(t, `^[a-z0-9-]+-[a-f0-9]{8}$`, result)
	})

	t.Run("respects prefix length to keep total under 63", func(t *testing.T) {
		// 模拟 prefix="xcron-" (6 字符) 的场景
		prefixLen := 6
		longName := "this-is-a-very-long-clean-name-that-is-exactly-at-the-boundary"
		result := sanitizeK8sName(longName, prefixLen)
		// 最终名称（prefix + result）不超过 63
		assert.LessOrEqual(t, prefixLen+len(result), 63)
	})

	t.Run("long clean name with default prefix stays under 63", func(t *testing.T) {
		// 实际使用场景：通过 leaseName 方法测试整体约束
		prefix := "xcron-"
		// 57 字符的 clean key（prefix 6 + key 57 = 63，刚好满足）
		longKey := "abcdefghijklmnopqrstuvwxyz-0123456789-abcdefghijklmnopqrst"
		result := prefix + sanitizeK8sName(longKey, len(prefix))
		assert.LessOrEqual(t, len(result), 63, "total lease name must not exceed 63 chars, got %d: %s", len(result), result)
	})

	t.Run("prevents collision from different separators", func(t *testing.T) {
		// 不同分隔符的名称清理后都变成 "my-job"，
		// 但 hash 后缀基于原始值，确保不同名称产生不同结果
		names := []string{"my.job", "my/job", "my:job", "my_job", "my job"}
		results := make(map[string]bool)
		for _, name := range names {
			result := sanitizeK8sName(name, 0)
			assert.False(t, results[result],
				"collision detected: %q produces duplicate result %q", name, result)
			results[result] = true
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		// 同一输入始终产生相同输出
		for i := 0; i < 10; i++ {
			assert.Equal(t, sanitizeK8sName("my.job", 0), sanitizeK8sName("my.job", 0))
		}
	})
}

func TestDefaultIdentity(t *testing.T) {
	identity := defaultIdentity()
	assert.NotEmpty(t, identity)
	// 格式应该是 hostname:pid
	assert.Contains(t, identity, ":")
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		result := getEnvOrDefault("XCRON_TEST_NOT_SET_VAR", "default-value")
		assert.Equal(t, "default-value", result)
	})

	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("XCRON_TEST_VAR", "env-value")
		result := getEnvOrDefault("XCRON_TEST_VAR", "default-value")
		assert.Equal(t, "env-value", result)
	})
}
