package xpool

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// newPoolForTest 创建 Pool，失败时终止测试。
func newPoolForTest[T any](t testing.TB, workers, queueSize int, handler func(T), opts ...Option) *Pool[T] {
	t.Helper()
	p, err := New(workers, queueSize, handler, opts...)
	require.NoError(t, err)
	return p
}

func TestWorkerPool_Basic(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)

	pool, err := New(2, 10, func(n int) {
		processed.Add(1)
		wg.Done()
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()

	for i := range 5 {
		err := pool.Submit(i)
		assert.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int32(5), processed.Load())
}

func TestWorkerPool_QueueFull(t *testing.T) {
	var processed atomic.Int32

	pool, err := New(1, 2, func(_ int) {
		time.Sleep(50 * time.Millisecond)
		processed.Add(1)
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()

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

func TestWorkerPool_Close(t *testing.T) {
	var processed atomic.Int32

	pool, err := New(2, 10, func(_ int) {
		processed.Add(1)
	})
	require.NoError(t, err)

	for i := range 5 {
		pool.Submit(i) // 测试中忽略提交错误
	}

	// 首次 Close 返回 nil
	require.NoError(t, pool.Close())

	// 关闭后提交应返回 ErrPoolStopped
	err = pool.Submit(100)
	assert.ErrorIs(t, err, ErrPoolStopped)

	// 多次 Close 返回 ErrPoolStopped
	assert.ErrorIs(t, pool.Close(), ErrPoolStopped)
	assert.ErrorIs(t, pool.Close(), ErrPoolStopped)
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	// 使用静默 logger 避免 panic 恢复日志污染测试输出。
	silentLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := New(1, 10, func(n int) {
		defer wg.Done()
		if n == 1 {
			panic("test panic")
		}
		processed.Add(1)
	}, WithLogger(silentLogger))
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()

	pool.Submit(0) // 测试中忽略提交错误
	pool.Submit(1) // 测试中忽略提交错误
	pool.Submit(2) // 测试中忽略提交错误

	wg.Wait()
	assert.Equal(t, int32(2), processed.Load())
}

func TestWorkerPool_InvalidConfig(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		queueSize int
		wantErr   error
	}{
		{"workers=0", 0, 10, ErrInvalidWorkers},
		{"workers=-1", -1, 10, ErrInvalidWorkers},
		{"workers too large", maxWorkers + 1, 10, ErrInvalidWorkers},
		{"workers=MaxInt", math.MaxInt, 10, ErrInvalidWorkers},
		{"queueSize=0", 1, 0, ErrInvalidQueueSize},
		{"queueSize=-1", 1, -1, ErrInvalidQueueSize},
		{"queueSize too large", 1, maxQueueSize + 1, ErrInvalidQueueSize},
		{"queueSize=MaxInt", 1, math.MaxInt, ErrInvalidQueueSize},
		// handler 校验在前，workers 次之
		{"both invalid returns first error", 0, 0, ErrInvalidWorkers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.workers, tt.queueSize, func(_ int) {})
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestWorkerPool_Concurrent(t *testing.T) {
	var processed atomic.Int64
	pool, err := New(4, 100, func(n int) {
		processed.Add(int64(n))
	})
	require.NoError(t, err)

	var submitted atomic.Int64
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 10 {
				if submitErr := pool.Submit(1); submitErr == nil {
					submitted.Add(1)
				}
			}
		})
	}

	wg.Wait()
	// Close 等待所有已入队任务处理完成（确定性同步，替代 time.Sleep）。
	require.NoError(t, pool.Close())

	// 断言精确处理数量：成功提交的任务必须全部被处理。
	assert.Equal(t, submitted.Load(), processed.Load())
	assert.Greater(t, submitted.Load(), int64(0), "至少应有部分任务成功提交")
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	var processed atomic.Int32

	pool, err := New(1, 100, func(_ int) {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
	})
	require.NoError(t, err)

	for i := range 10 {
		pool.Submit(i) // 测试中忽略提交错误
	}

	require.NoError(t, pool.Close())

	assert.Equal(t, int32(10), processed.Load())
}

func TestWorkerPool_Accessors(t *testing.T) {
	pool := newPoolForTest(t, 3, 50, func(_ int) {})
	defer func() { require.NoError(t, pool.Close()) }()

	assert.Equal(t, 3, pool.Workers())
	assert.Equal(t, 50, pool.QueueSize())
	assert.Equal(t, 0, pool.QueueLen())
}

func TestWorkerPool_NilHandler(t *testing.T) {
	_, err := New[int](2, 10, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestWorkerPool_WithLogger(t *testing.T) {
	logger := slog.Default()
	pool, err := New(1, 10, func(_ int) {},
		WithLogger(logger),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()
}

func TestWorkerPool_WithNilLogger(t *testing.T) {
	// nil logger 不应覆盖默认值
	pool, err := New(1, 10, func(_ int) {},
		WithLogger(nil),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()
}

func TestWorkerPool_WithName(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	pool := newPoolForTest(t, 1, 10, func(n int) {
		if n == 0 {
			panic("named pool panic")
		}
	}, WithLogger(logger), WithName("test-pool"))

	require.NoError(t, pool.Submit(0))

	// Close 同步等待所有任务完成，确保 panic 日志已写入 buf。
	require.NoError(t, pool.Close())

	logOutput := buf.String()
	assert.Contains(t, logOutput, "pool=test-pool", "log should contain pool name")
}

func TestWorkerPool_WithNameEmpty(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	pool := newPoolForTest(t, 1, 10, func(n int) {
		if n == 0 {
			panic("unnamed pool panic")
		}
	}, WithLogger(logger))

	require.NoError(t, pool.Submit(0))

	// Close 同步等待所有任务完成，确保 panic 日志已写入 buf。
	require.NoError(t, pool.Close())

	logOutput := buf.String()
	assert.NotContains(t, logOutput, "pool=", "log should not contain pool key when name is empty")
}

func TestWorkerPool_NilOption(t *testing.T) {
	pool, err := New(1, 10, func(_ int) {}, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()
}

func TestWorkerPool_SubmitAfterClose(t *testing.T) {
	pool, err := New(2, 10, func(_ int) {})
	require.NoError(t, err)

	require.NoError(t, pool.Close())

	err = pool.Submit(42)
	assert.ErrorIs(t, err, ErrPoolStopped)
}

func TestWorkerPool_ConcurrentSubmitAndClose(t *testing.T) {
	pool, err := New(4, 100, func(_ int) {
		time.Sleep(time.Millisecond)
	})
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 并发提交
	for range 10 {
		wg.Go(func() {
			for j := range 100 {
				pool.Submit(j) // 测试中忽略提交错误
			}
		})
	}

	// 并发关闭
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond)
		pool.Close() // 并发关闭测试
	})

	wg.Wait()
}

func TestWorkerPool_QueueLen(t *testing.T) {
	// 使用阻塞 handler 让任务积压在队列中。
	// started 必须带缓冲：handler 的 select/default 在主 goroutine 尚未到达
	// <-started 时会走 default 分支丢弃信号，导致 race 模式下死锁。
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	pool, err := New(1, 10, func(_ int) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pool.Close()) }()

	// 初始队列为空
	assert.Equal(t, 0, pool.QueueLen())

	// 提交 5 个任务
	for i := range 5 {
		require.NoError(t, pool.Submit(i))
	}

	// 等待第一个任务被 worker 取走并开始执行
	<-started

	// 队列中剩余 4 个（1 个正在执行不在队列中）
	assert.Equal(t, 4, pool.QueueLen())

	// 释放所有任务
	close(release)
}

func TestWorkerPool_ZeroValue(t *testing.T) {
	var p Pool[int]

	// 零值 Done 应立即就绪（无 worker 需等待）
	select {
	case <-p.Done():
	default:
		t.Fatal("zero-value Pool.Done() should be immediately ready")
	}

	// 零值 Submit 返回 ErrPoolStopped（非 panic）
	assert.ErrorIs(t, p.Submit(42), ErrPoolStopped)

	// 零值 Close 不 panic
	assert.NoError(t, p.Close())

	// 再次 Close 返回 ErrPoolStopped
	assert.ErrorIs(t, p.Close(), ErrPoolStopped)

	// Close 后 Submit 返回 ErrPoolStopped
	assert.ErrorIs(t, p.Submit(42), ErrPoolStopped)

	// Accessors 不 panic
	assert.Equal(t, 0, p.Workers())
	assert.Equal(t, 0, p.QueueSize())
	assert.Equal(t, 0, p.QueueLen())
}

func TestWorkerPool_Shutdown(t *testing.T) {
	var processed atomic.Int32

	pool := newPoolForTest(t, 2, 10, func(_ int) {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
	})

	for i := range 5 {
		require.NoError(t, pool.Submit(i))
	}

	// 充裕超时：所有任务应完成
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, pool.Shutdown(ctx))
	assert.Equal(t, int32(5), processed.Load())

	// 再次调用返回 ErrPoolStopped
	assert.ErrorIs(t, pool.Shutdown(context.Background()), ErrPoolStopped)
}

func TestWorkerPool_ShutdownTimeout(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	pool := newPoolForTest(t, 1, 10, func(_ int) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
	})

	require.NoError(t, pool.Submit(0))
	<-started // 等待 worker 开始处理

	// 极短超时：worker 阻塞中，应返回 DeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := pool.Shutdown(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// 释放 worker，通过 Done() 等待 worker 退出（避免 goleak）
	close(release)
	select {
	case <-pool.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() should close after workers finish")
	}
}

func TestWorkerPool_ShutdownNilContext(t *testing.T) {
	pool := newPoolForTest(t, 1, 10, func(_ int) {})
	defer func() { require.NoError(t, pool.Close()) }()

	var nilCtx context.Context // 故意使用 nil context 测试错误返回
	err := pool.Shutdown(nilCtx)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestWorkerPool_ShutdownAlreadyCancelled(t *testing.T) {
	pool := newPoolForTest(t, 1, 10, func(_ int) {})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := pool.Shutdown(ctx)
	// 已取消的 ctx：队列已关闭但不等待 worker，返回 ctx 错误
	assert.ErrorIs(t, err, context.Canceled)

	// worker 仍在后台运行，通过 Done() 等待退出
	select {
	case <-pool.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() should close after workers finish")
	}
}

func TestWorkerPool_ConcurrentShutdown(t *testing.T) {
	pool := newPoolForTest(t, 2, 10, func(_ int) {})

	const n = 10
	results := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			results <- pool.Shutdown(context.Background())
		})
	}
	wg.Wait()
	close(results)

	var nilCount, stoppedCount int
	for err := range results {
		switch err {
		case nil:
			nilCount++
		default:
			assert.ErrorIs(t, err, ErrPoolStopped)
			stoppedCount++
		}
	}
	assert.Equal(t, 1, nilCount, "exactly one Shutdown should return nil")
	assert.Equal(t, n-1, stoppedCount, "all others should return ErrPoolStopped")
}

func TestWorkerPool_DoneBeforeShutdown(t *testing.T) {
	pool := newPoolForTest(t, 1, 10, func(_ int) {})
	defer func() { require.NoError(t, pool.Close()) }()

	// Done() 在 Shutdown 前不应就绪
	select {
	case <-pool.Done():
		t.Fatal("Done() should not be ready before Shutdown")
	default:
	}
}

func TestWorkerPool_PanicRecoveryNonString(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)

	silentLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := newPoolForTest(t, 1, 10, func(n int) {
		defer wg.Done()
		if n == 0 {
			panic(42) // 非字符串 panic
		}
	}, WithLogger(silentLogger))
	defer func() { require.NoError(t, pool.Close()) }()

	require.NoError(t, pool.Submit(0)) // panic(42)
	require.NoError(t, pool.Submit(1)) // 正常处理

	wg.Wait()
}

func TestWorkerPool_DoneAfterClose(t *testing.T) {
	pool := newPoolForTest(t, 2, 10, func(_ int) {})
	require.NoError(t, pool.Close())

	select {
	case <-pool.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() should be closed after Close completes")
	}
}

func TestWorkerPool_DoneAfterShutdownTimeout(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	pool := newPoolForTest(t, 1, 10, func(_ int) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
	})

	require.NoError(t, pool.Submit(0))
	<-started

	// 极短超时：worker 阻塞中，应返回 DeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := pool.Shutdown(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Done() 尚未就绪（worker 仍阻塞）
	select {
	case <-pool.Done():
		t.Fatal("Done() should not be ready while worker is blocked")
	default:
	}

	// 释放 worker
	close(release)

	// Done() 应在 worker 完成后就绪
	select {
	case <-pool.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() should close after workers finish")
	}
}
