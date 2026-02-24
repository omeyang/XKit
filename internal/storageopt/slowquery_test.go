package storageopt

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSlowQueryInfo 测试用的慢查询信息结构
type testSlowQueryInfo struct {
	Query    string
	Duration time.Duration
}

func TestSlowQueryDetector_Disabled(t *testing.T) {
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 0, // 禁用
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "test", Duration: 1 * time.Hour}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.False(t, triggered)
}

func TestSlowQueryDetector_BelowThreshold(t *testing.T) {
	var called bool
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			called = true
		},
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "test", Duration: 50 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.False(t, triggered)
	assert.False(t, called)
}

func TestSlowQueryDetector_SyncHook(t *testing.T) {
	var called bool
	var captured testSlowQueryInfo

	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			called = true
			captured = info
		},
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "SELECT * FROM test", Duration: 200 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.True(t, triggered)
	assert.True(t, called)
	assert.Equal(t, "SELECT * FROM test", captured.Query)
	assert.Equal(t, 200*time.Millisecond, captured.Duration)
}

func TestSlowQueryDetector_AsyncHook(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	var captured testSlowQueryInfo
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		AsyncHook: func(info testSlowQueryInfo) {
			captured = info
			wg.Done()
		},
		AsyncWorkerPoolSize: 1,
		AsyncQueueSize:      10,
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "SELECT * FROM test", Duration: 200 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.True(t, triggered)

	// 等待异步处理完成
	wg.Wait()
	assert.Equal(t, "SELECT * FROM test", captured.Query)
}

func TestSlowQueryDetector_BothHooks(t *testing.T) {
	var syncCalled, asyncCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			syncCalled.Store(true)
		},
		AsyncHook: func(info testSlowQueryInfo) {
			asyncCalled.Store(true)
			wg.Done()
		},
		AsyncWorkerPoolSize: 1,
		AsyncQueueSize:      10,
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "test", Duration: 200 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.True(t, triggered)
	assert.True(t, syncCalled.Load())

	// 等待异步处理
	wg.Wait()
	assert.True(t, asyncCalled.Load())
}

func TestSlowQueryDetector_ExactThreshold(t *testing.T) {
	var called bool
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			called = true
		},
	})
	require.NoError(t, err)
	defer detector.Close()

	// 刚好等于阈值，应该触发（使用 >= 比较）
	info := testSlowQueryInfo{Query: "test", Duration: 100 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.True(t, triggered)
	assert.True(t, called)
}

func TestSlowQueryDetector_JustAboveThreshold(t *testing.T) {
	var called bool
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			called = true
		},
	})
	require.NoError(t, err)
	defer detector.Close()

	// 刚好超过阈值
	info := testSlowQueryInfo{Query: "test", Duration: 101 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	assert.True(t, triggered)
	assert.True(t, called)
}

func TestSlowQueryDetector_Close(t *testing.T) {
	var count atomic.Int32
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 1 * time.Nanosecond,
		AsyncHook: func(info testSlowQueryInfo) {
			count.Add(1)
		},
		AsyncWorkerPoolSize: 1,
		AsyncQueueSize:      10,
	})
	require.NoError(t, err)

	// 触发一些慢查询
	for i := 0; i < 5; i++ {
		detector.MaybeSlowQuery(context.Background(), testSlowQueryInfo{Duration: time.Hour}, time.Hour)
	}

	// 关闭
	detector.Close()

	// 多次关闭应该是安全的
	detector.Close()
	detector.Close()

	// 关闭后再触发不应该 panic
	detector.MaybeSlowQuery(context.Background(), testSlowQueryInfo{Duration: time.Hour}, time.Hour)
}

func TestSlowQueryDetector_DefaultOptions(t *testing.T) {
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
		AsyncHook: func(info testSlowQueryInfo) {},
		// 不设置 pool size 和 queue size
	})
	require.NoError(t, err)
	defer detector.Close()

	// 应该使用默认值
	assert.Equal(t, DefaultAsyncWorkerPoolSize, detector.options.AsyncWorkerPoolSize)
	assert.Equal(t, DefaultAsyncQueueSize, detector.options.AsyncQueueSize)
}

func TestSlowQueryDetector_Concurrent(t *testing.T) {
	var syncCount, asyncCount atomic.Int32

	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 1 * time.Nanosecond,
		SyncHook: func(ctx context.Context, info testSlowQueryInfo) {
			syncCount.Add(1)
		},
		AsyncHook: func(info testSlowQueryInfo) {
			asyncCount.Add(1)
		},
		AsyncWorkerPoolSize: 4,
		AsyncQueueSize:      100,
	})
	require.NoError(t, err)
	defer detector.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				detector.MaybeSlowQuery(context.Background(),
					testSlowQueryInfo{Duration: time.Hour}, time.Hour)
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 所有同步调用都应该完成
	assert.Equal(t, int32(100), syncCount.Load())
	// 异步调用可能有些被丢弃，但应该有大部分
	assert.Greater(t, asyncCount.Load(), int32(0))
}

func TestSlowQueryDetector_NoHooks(t *testing.T) {
	// 只有阈值，没有钩子
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	defer detector.Close()

	info := testSlowQueryInfo{Query: "test", Duration: 200 * time.Millisecond}
	triggered := detector.MaybeSlowQuery(context.Background(), info, info.Duration)

	// 超过阈值，但没有钩子，仍返回 true
	assert.True(t, triggered)
}

func TestSlowQueryDetector_ConcurrentCloseAndQuery(t *testing.T) {
	detector, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold:           1 * time.Nanosecond,
		AsyncHook:           func(info testSlowQueryInfo) {},
		AsyncWorkerPoolSize: 2,
		AsyncQueueSize:      100,
	})
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 多个 goroutine 并发调用 MaybeSlowQuery
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				detector.MaybeSlowQuery(context.Background(),
					testSlowQueryInfo{Duration: time.Hour}, time.Hour)
			}
		}()
	}

	// 另一个 goroutine 并发调用 Close
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		detector.Close()
	}()

	wg.Wait()
}

func TestNewSlowQueryDetector_InvalidPoolParams(t *testing.T) {
	// 超大 worker pool 应该返回错误
	_, err := NewSlowQueryDetector(SlowQueryOptions[testSlowQueryInfo]{
		Threshold:           100 * time.Millisecond,
		AsyncHook:           func(info testSlowQueryInfo) {},
		AsyncWorkerPoolSize: 1 << 17, // 超出 xpool 上限
		AsyncQueueSize:      10,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storageopt: create async pool")
}
