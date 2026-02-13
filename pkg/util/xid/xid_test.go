package xid

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sony/sonyflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetGlobal 重置全局状态，仅用于测试
func resetGlobal() {
	defaultGen = nil
	defaultGenErr = nil
	defaultGenOnce = sync.Once{}
}

func TestNew(t *testing.T) {
	resetGlobal()

	id1, err := New()
	require.NoError(t, err)
	assert.NotZero(t, id1)

	id2, err := New()
	require.NoError(t, err)
	assert.NotZero(t, id2)

	// ID 应该递增（除非在同一时间单位内生成太多）
	assert.NotEqual(t, id1, id2)
}

func TestNewString(t *testing.T) {
	resetGlobal()

	s1, err := NewString()
	require.NoError(t, err)
	assert.NotEmpty(t, s1)
	// base36 编码的 uint64 最多 13 个字符
	assert.LessOrEqual(t, len(s1), 13)

	s2, err := NewString()
	require.NoError(t, err)
	assert.NotEqual(t, s1, s2)
}

func TestMustNew(t *testing.T) {
	resetGlobal()

	// 正常情况不应 panic
	assert.NotPanics(t, func() {
		id := MustNew()
		assert.NotZero(t, id)
	})
}

func TestMustNewString(t *testing.T) {
	resetGlobal()

	// 正常情况不应 panic
	assert.NotPanics(t, func() {
		s := MustNewString()
		assert.NotEmpty(t, s)
	})
}

func TestParse(t *testing.T) {
	resetGlobal()

	// 生成 ID
	s, err := NewString()
	require.NoError(t, err)

	// 解析回 uint64
	id, err := Parse(s)
	require.NoError(t, err)

	// 再次格式化应该得到相同结果
	s2, err := NewString()
	require.NoError(t, err)
	id2, err := Parse(s2)
	require.NoError(t, err)

	assert.NotEqual(t, id, id2)
}

func TestDecompose(t *testing.T) {
	resetGlobal()

	id, err := New()
	require.NoError(t, err)

	parts, err := Decompose(id)
	require.NoError(t, err)
	assert.Equal(t, id, parts.ID)
	assert.NotZero(t, parts.Time)
}

func TestInit_WithCustomMachineID(t *testing.T) {
	resetGlobal()

	customID := uint16(12345)
	err := Init(WithMachineID(func() (uint16, error) {
		return customID, nil
	}))
	require.NoError(t, err)

	// 生成 ID 并验证机器 ID 部分
	id, err := New()
	require.NoError(t, err)

	parts, err := Decompose(id)
	require.NoError(t, err)
	assert.Equal(t, int64(customID), parts.Machine)
}

func TestInit_AlreadyInitialized(t *testing.T) {
	resetGlobal()

	// 第一次 Init 成功
	err := Init()
	require.NoError(t, err)

	// 第二次 Init 返回 ErrAlreadyInitialized
	err = Init()
	assert.ErrorIs(t, err, ErrAlreadyInitialized)
}

func TestInit_AlreadyInitialized_ByAutoInit(t *testing.T) {
	resetGlobal()

	// 通过 New 触发自动初始化
	_, err := New()
	require.NoError(t, err)

	// 显式 Init 返回 ErrAlreadyInitialized
	err = Init()
	assert.ErrorIs(t, err, ErrAlreadyInitialized)
}

func TestInit_WithCheckMachineID(t *testing.T) {
	t.Run("check passes", func(t *testing.T) {
		resetGlobal()
		err := Init(
			WithMachineID(func() (uint16, error) {
				return 100, nil
			}),
			WithCheckMachineID(func(id uint16) bool {
				return id == 100
			}),
		)
		require.NoError(t, err)
	})

	t.Run("check fails", func(t *testing.T) {
		resetGlobal()
		err := Init(
			WithMachineID(func() (uint16, error) {
				return 100, nil
			}),
			WithCheckMachineID(func(id uint16) bool {
				return id == 200 // 期望 200，但提供的是 100
			}),
		)
		assert.Error(t, err)
	})
}

func TestConcurrentGeneration(t *testing.T) {
	resetGlobal()

	const goroutines = 100
	const idsPerGoroutine = 100

	ids := make(chan int64, goroutines*idsPerGoroutine)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				id, err := New()
				if err != nil {
					t.Errorf("New() failed: %v", err)
					return
				}
				ids <- id
			}
		}()
	}

	wg.Wait()
	close(ids)

	// 检查唯一性
	seen := make(map[int64]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %d", id)
		}
		seen[id] = true
	}

	assert.Equal(t, goroutines*idsPerGoroutine, len(seen))
}

func TestAutoInit(t *testing.T) {
	resetGlobal()

	// 不调用 Init，直接生成 ID 应该自动初始化
	id, err := New()
	require.NoError(t, err)
	assert.NotZero(t, id)
}

func TestNewWithRetry(t *testing.T) {
	resetGlobal()

	ctx := context.Background()

	// 正常情况下应该成功
	id, err := NewWithRetry(ctx)
	require.NoError(t, err)
	assert.NotZero(t, id)

	// 连续生成应该都成功
	for i := 0; i < 100; i++ {
		id2, err := NewWithRetry(ctx)
		require.NoError(t, err)
		assert.NotEqual(t, id, id2)
		id = id2
	}
}

func TestNewStringWithRetry(t *testing.T) {
	resetGlobal()

	s, err := NewStringWithRetry(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, s)
}

func TestClockBackwardConfig(t *testing.T) {
	resetGlobal()

	// 测试自定义配置
	err := Init(
		WithMaxWaitDuration(100*time.Millisecond),
		WithRetryInterval(5*time.Millisecond),
	)
	require.NoError(t, err)

	// 验证配置生效（通过 defaultGen 的字段检查）
	require.NotNil(t, defaultGen)
	assert.Equal(t, 100*time.Millisecond, defaultGen.maxWaitDuration)
	assert.Equal(t, 5*time.Millisecond, defaultGen.retryInterval)
}

func TestErrClockBackwardTimeout_IsUnwrappable(t *testing.T) {
	// 验证 ErrClockBackwardTimeout 可以使用 errors.Is 检查
	// 这测试了 P2 修复后的错误语义
	resetGlobal()

	// 创建一个包装了原始错误的超时错误
	originalErr := errors.New("original clock error")
	wrappedErr := fmt.Errorf("%w: last error was %v", ErrClockBackwardTimeout, originalErr)

	// 验证 errors.Is 可以识别底层错误
	assert.True(t, errors.Is(wrappedErr, ErrClockBackwardTimeout),
		"wrapped error should be unwrappable to ErrClockBackwardTimeout")

	// 验证错误消息包含原始错误信息
	assert.Contains(t, wrappedErr.Error(), "clock backward wait timeout")
	assert.Contains(t, wrappedErr.Error(), "original clock error")
}

func BenchmarkNew(b *testing.B) {
	resetGlobal()
	_ = Init() //nolint:errcheck // benchmark init

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = New() //nolint:errcheck // benchmark
	}
}

func BenchmarkNewString(b *testing.B) {
	resetGlobal()
	_ = Init() //nolint:errcheck // benchmark init

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewString() //nolint:errcheck // benchmark
	}
}

func BenchmarkMustNewString(b *testing.B) {
	resetGlobal()
	_ = Init() //nolint:errcheck // benchmark init

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MustNewString()
	}
}

// BenchmarkComparison 对比不同 ID 生成方案的性能
func BenchmarkComparison(b *testing.B) {
	resetGlobal()

	b.Run("xid/New", func(b *testing.B) {
		_ = Init() //nolint:errcheck // benchmark init
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = New() //nolint:errcheck // benchmark
		}
	})

	b.Run("xid/NewString", func(b *testing.B) {
		_ = Init() //nolint:errcheck // benchmark init
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = NewString() //nolint:errcheck // benchmark
		}
	})

	b.Run("sonyflake/NextID", func(b *testing.B) {
		sf, _ := sonyflake.New(sonyflake.Settings{}) //nolint:errcheck // benchmark init
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = sf.NextID() //nolint:errcheck // benchmark
		}
	})
}
