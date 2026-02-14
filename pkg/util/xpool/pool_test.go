package xpool

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerPool_Basic(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)

	pool, err := NewWorkerPool(2, 10, func(n int) {
		processed.Add(1)
		wg.Done()
	})
	require.NoError(t, err)
	defer pool.Stop()

	for i := range 5 {
		err := pool.Submit(i)
		assert.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int32(5), processed.Load())
}

func TestWorkerPool_QueueFull(t *testing.T) {
	var processed atomic.Int32

	pool, err := NewWorkerPool(1, 2, func(_ int) {
		time.Sleep(50 * time.Millisecond)
		processed.Add(1)
	})
	require.NoError(t, err)
	defer pool.Stop()

	var submitted, dropped int
	for i := range 10 {
		if err := pool.Submit(i); err == nil {
			submitted++
		} else {
			assert.ErrorIs(t, err, ErrQueueFull)
			dropped++
		}
	}

	t.Logf("submitted: %d, dropped: %d", submitted, dropped)
	assert.Greater(t, submitted, 0)
	assert.Greater(t, dropped, 0)

	time.Sleep(200 * time.Millisecond)
	assert.Greater(t, processed.Load(), int32(0))
}

func TestWorkerPool_Stop(t *testing.T) {
	var processed atomic.Int32

	pool, err := NewWorkerPool(2, 10, func(_ int) {
		processed.Add(1)
	})
	require.NoError(t, err)

	for i := range 5 {
		pool.Submit(i) //nolint:errcheck // 测试中忽略提交错误
	}

	pool.Stop()

	// 停止后提交应返回 ErrPoolStopped
	err = pool.Submit(100)
	assert.ErrorIs(t, err, ErrPoolStopped)

	// 多次 Stop 应该是安全的
	pool.Stop()
	pool.Stop()
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	pool, err := NewWorkerPool(1, 10, func(n int) {
		defer wg.Done()
		if n == 1 {
			panic("test panic")
		}
		processed.Add(1)
	})
	require.NoError(t, err)
	defer pool.Stop()

	pool.Submit(0) //nolint:errcheck // 测试中忽略提交错误
	pool.Submit(1) //nolint:errcheck // 测试中忽略提交错误
	pool.Submit(2) //nolint:errcheck // 测试中忽略提交错误

	wg.Wait()
	assert.Equal(t, int32(2), processed.Load())
}

func TestWorkerPool_DefaultValues(t *testing.T) {
	pool, err := NewWorkerPool(0, 0, func(_ int) {})
	require.NoError(t, err)
	defer pool.Stop()

	assert.Equal(t, 1, pool.Workers())
	assert.Equal(t, 100, pool.QueueSize())
}

func TestWorkerPool_Concurrent(t *testing.T) {
	var processed atomic.Int64
	pool, err := NewWorkerPool(4, 100, func(n int) {
		processed.Add(int64(n))
	})
	require.NoError(t, err)
	defer pool.Stop()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 10 {
				pool.Submit(1) //nolint:errcheck // 测试中忽略提交错误
			}
		})
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	assert.Greater(t, processed.Load(), int64(0))
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	var processed atomic.Int32

	pool, err := NewWorkerPool(1, 100, func(_ int) {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
	})
	require.NoError(t, err)

	for i := range 10 {
		pool.Submit(i) //nolint:errcheck // 测试中忽略提交错误
	}

	pool.Stop()

	assert.Equal(t, int32(10), processed.Load())
}

func TestWorkerPool_NilHandler(t *testing.T) {
	_, err := NewWorkerPool[int](2, 10, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestWorkerPool_WithLogger(t *testing.T) {
	logger := slog.Default()
	pool, err := NewWorkerPool(1, 10, func(_ int) {},
		WithLogger[int](logger),
	)
	require.NoError(t, err)
	defer pool.Stop()
}

func TestWorkerPool_WithNilLogger(t *testing.T) {
	// nil logger 不应覆盖默认值
	pool, err := NewWorkerPool(1, 10, func(_ int) {},
		WithLogger[int](nil),
	)
	require.NoError(t, err)
	defer pool.Stop()
}

func TestWorkerPool_NilOption(t *testing.T) {
	pool, err := NewWorkerPool(1, 10, func(_ int) {}, nil)
	require.NoError(t, err)
	defer pool.Stop()
}

func TestWorkerPool_SubmitAfterStop(t *testing.T) {
	pool, err := NewWorkerPool(2, 10, func(_ int) {})
	require.NoError(t, err)

	pool.Stop()

	err = pool.Submit(42)
	assert.ErrorIs(t, err, ErrPoolStopped)
}

func TestWorkerPool_ConcurrentSubmitAndStop(t *testing.T) {
	pool, err := NewWorkerPool(4, 100, func(_ int) {
		time.Sleep(time.Millisecond)
	})
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 并发提交
	for range 10 {
		wg.Go(func() {
			for j := range 100 {
				pool.Submit(j) //nolint:errcheck // 测试中忽略提交错误
			}
		})
	}

	// 并发停止
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond)
		pool.Stop()
	})

	wg.Wait()
}
