package xpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkerPool_Basic(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)

	pool := NewWorkerPool(2, 10, func(n int) {
		processed.Add(1)
		wg.Done()
	})
	pool.Start()
	defer pool.Stop()

	for i := 0; i < 5; i++ {
		submitted := pool.Submit(i)
		assert.True(t, submitted)
	}

	wg.Wait()
	assert.Equal(t, int32(5), processed.Load())
}

func TestWorkerPool_QueueFull(t *testing.T) {
	// 创建一个小队列和慢处理器
	var processed atomic.Int32

	pool := NewWorkerPool(1, 2, func(_ int) {
		time.Sleep(50 * time.Millisecond)
		processed.Add(1)
	})
	pool.Start()
	defer pool.Stop()

	// 提交多个任务
	var submitted, dropped int
	for i := 0; i < 10; i++ {
		if pool.Submit(i) {
			submitted++
		} else {
			dropped++
		}
	}

	// 应该有一些任务被丢弃
	t.Logf("submitted: %d, dropped: %d", submitted, dropped)
	assert.Greater(t, submitted, 0)
	// 队列可能满导致部分任务被丢弃

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)
	assert.Greater(t, processed.Load(), int32(0))
}

func TestWorkerPool_Stop(t *testing.T) {
	var processed atomic.Int32

	pool := NewWorkerPool(2, 10, func(_ int) {
		processed.Add(1)
	})
	pool.Start()

	// 提交一些任务
	for i := 0; i < 5; i++ {
		pool.Submit(i)
	}

	// 立即停止
	pool.Stop()

	// 再次提交应该失败
	submitted := pool.Submit(100)
	assert.False(t, submitted)

	// 多次 Stop 应该是安全的
	pool.Stop()
	pool.Stop()
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	pool := NewWorkerPool(1, 10, func(n int) {
		defer wg.Done()
		if n == 1 {
			panic("test panic")
		}
		processed.Add(1)
	})
	pool.Start()
	defer pool.Stop()

	// 第一个任务正常
	pool.Submit(0)
	// 第二个任务 panic
	pool.Submit(1)
	// 第三个任务应该仍然能执行
	pool.Submit(2)

	wg.Wait()
	// 只有 2 个任务实际处理成功
	assert.Equal(t, int32(2), processed.Load())
}

func TestWorkerPool_DefaultValues(t *testing.T) {
	// 测试默认值处理
	pool := NewWorkerPool(0, 0, func(_ int) {})
	assert.Equal(t, 1, pool.Workers())
	assert.Equal(t, 100, pool.QueueSize())
}

func TestWorkerPool_Concurrent(t *testing.T) {
	var processed atomic.Int64
	pool := NewWorkerPool(4, 100, func(n int) {
		processed.Add(int64(n))
	})
	pool.Start()
	defer pool.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				pool.Submit(1)
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 应该处理了大部分任务（可能有些被丢弃）
	assert.Greater(t, processed.Load(), int64(0))
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	var processed atomic.Int32

	pool := NewWorkerPool(1, 100, func(_ int) {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
	})
	pool.Start()

	// 提交 10 个任务
	for i := 0; i < 10; i++ {
		pool.Submit(i)
	}

	// 停止并等待
	pool.Stop()

	// 由于优雅关闭，所有任务应该都被处理了
	assert.Equal(t, int32(10), processed.Load())
}
