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

func TestSanitizeK8sName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"WITH_UPPER", "with-upper"},
		{"with.dot", "with-dot"},
		{"with/slash", "with-slash"},
		{"with:colon", "with-colon"},
		{"with space", "with-space"},
		{"multiple---dashes", "multiple-dashes"},
		{"-leading-dash", "leading-dash"},
		{"trailing-dash-", "trailing-dash"},
		{"", ""},
		{"a", "a"},
		{"a-b-c", "a-b-c"},
		{"123", "123"},
		{"a1b2c3", "a1b2c3"},
		// 长名称会被截断并添加 hash 后缀确保唯一性
		{"this-is-a-very-long-name-that-exceeds-the-maximum-length-allowed-by-kubernetes-for-resource-names", "this-is-a-very-long-name-that-exceeds-the-maximum-leng-410105ea"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeK8sName(tt.input)
			assert.Equal(t, tt.expected, result)
			// 验证长度不超过 63
			assert.LessOrEqual(t, len(result), 63)
		})
	}
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
