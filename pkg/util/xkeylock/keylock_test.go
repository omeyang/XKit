package xkeylock

import (
	"context"
	"fmt"
	"runtime"
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

// newForTest 创建 Locker，失败时终止测试。
func newForTest(tb testing.TB, opts ...Option) Locker {
	tb.Helper()
	kl, err := New(opts...)
	require.NoError(tb, err)
	return kl
}

func TestAcquireEmptyKey(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	_, err := kl.Acquire(context.Background(), "")
	assert.ErrorIs(t, err, ErrInvalidKey)

	_, err = kl.TryAcquire("")
	assert.ErrorIs(t, err, ErrInvalidKey)

	// 确认没有残留 entry
	assert.Equal(t, 0, kl.Len())
}

func TestAcquireNilContext(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	_, err := kl.Acquire(nil, "key1") //nolint:staticcheck // 故意传入 nil ctx 以验证 ErrNilContext
	assert.ErrorIs(t, err, ErrNilContext)

	// 确认没有残留 entry
	assert.Equal(t, 0, kl.Len())
}

func TestAcquireAndUnlock(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)
	require.NotNil(t, h)
	assert.Equal(t, "key1", h.Key())

	assert.NoError(t, h.Unlock())
}

func TestUnlockIdempotent(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	// First unlock succeeds
	assert.NoError(t, h.Unlock())

	// Second unlock returns ErrLockNotHeld
	assert.ErrorIs(t, h.Unlock(), ErrLockNotHeld)

	// Third unlock also returns ErrLockNotHeld
	assert.ErrorIs(t, h.Unlock(), ErrLockNotHeld)
}

func TestTryAcquire(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	// First acquire succeeds
	h1, err := kl.TryAcquire("key1")
	require.NoError(t, err)
	require.NotNil(t, h1)

	// Second acquire fails (lock held)
	h2, err := kl.TryAcquire("key1")
	assert.ErrorIs(t, err, ErrLockOccupied)
	assert.Nil(t, h2)

	// Different key succeeds
	h3, err := kl.TryAcquire("key2")
	require.NoError(t, err)
	require.NotNil(t, h3)

	// Unlock key1, then try again
	require.NoError(t, h1.Unlock())
	h4, err := kl.TryAcquire("key1")
	require.NoError(t, err)
	require.NotNil(t, h4)

	require.NoError(t, h3.Unlock())
	require.NoError(t, h4.Unlock())
}

func TestAcquireContextCancel(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	// Hold the lock
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	// Try to acquire with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = kl.Acquire(ctx, "key1")
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	require.NoError(t, h.Unlock())
}

func TestAcquireAfterClose(t *testing.T) {
	kl := newForTest(t)
	require.NoError(t, kl.Close())

	_, err := kl.Acquire(context.Background(), "key1")
	assert.ErrorIs(t, err, ErrClosed)

	_, err = kl.TryAcquire("key1")
	assert.ErrorIs(t, err, ErrClosed)
}

func TestCloseIdempotent(t *testing.T) {
	kl := newForTest(t)
	assert.NoError(t, kl.Close())
	assert.ErrorIs(t, kl.Close(), ErrClosed)
}

func TestCloseDoesNotAffectHeldLocks(t *testing.T) {
	kl := newForTest(t)

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	require.NoError(t, kl.Close())

	// Unlock still works
	assert.NoError(t, h.Unlock())
}

func TestKeys(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	h1, err := kl.Acquire(context.Background(), "a")
	require.NoError(t, err)
	h2, err := kl.Acquire(context.Background(), "b")
	require.NoError(t, err)

	keys := kl.Keys()
	assert.ElementsMatch(t, []string{"a", "b"}, keys)

	require.NoError(t, h1.Unlock())
	require.NoError(t, h2.Unlock())

	// After unlock, keys should eventually be cleaned up
	// Keys list entries with refcnt > 0, which are now cleaned up
	keys = kl.Keys()
	assert.Empty(t, keys)
}

func TestMaxKeys(t *testing.T) {
	kl := newForTest(t, WithMaxKeys(2))
	defer func() { require.NoError(t, kl.Close()) }()

	h1, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)
	h2, err := kl.Acquire(context.Background(), "key2")
	require.NoError(t, err)

	// Third key should fail
	_, err = kl.Acquire(context.Background(), "key3")
	assert.ErrorIs(t, err, ErrMaxKeysExceeded)

	_, err = kl.TryAcquire("key3")
	assert.ErrorIs(t, err, ErrMaxKeysExceeded)

	// Release one, then acquire new key
	require.NoError(t, h1.Unlock())
	h3, err := kl.Acquire(context.Background(), "key3")
	require.NoError(t, err)

	require.NoError(t, h2.Unlock())
	require.NoError(t, h3.Unlock())
}

func TestShardCount(t *testing.T) {
	// 有效分片数
	kl := newForTest(t, WithShardCount(64))
	impl, ok := kl.(*keyLockImpl)
	require.True(t, ok)
	assert.Equal(t, 64, len(impl.shards))
	require.NoError(t, kl.Close())

	// 最小有效分片数（单分片，退化为全局锁）
	kl1 := newForTest(t, WithShardCount(1))
	impl1, ok1 := kl1.(*keyLockImpl)
	require.True(t, ok1)
	assert.Equal(t, 1, len(impl1.shards))
	require.NoError(t, kl1.Close())

	// 最大有效分片数
	kl2 := newForTest(t, WithShardCount(maxShardCount))
	impl2, ok2 := kl2.(*keyLockImpl)
	require.True(t, ok2)
	assert.Equal(t, maxShardCount, len(impl2.shards))
	require.NoError(t, kl2.Close())

	invalidCases := []struct {
		name  string
		count int
	}{
		{"not power of 2", 3},
		{"zero", 0},
		{"negative", -1},
		{"exceeds max", maxShardCount * 2},
		{"extreme value", 1 << 30},
	}
	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(WithShardCount(tt.count))
			assert.ErrorIs(t, err, ErrInvalidShardCount)
			assert.Contains(t, err.Error(), "power of 2")
		})
	}
}

func TestConcurrentMutualExclusion(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	const (
		numGoroutines = 50
		numIterations = 100
	)

	var counter int64
	var wg sync.WaitGroup
	var violations atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				h, err := kl.Acquire(context.Background(), "shared-key")
				if err != nil {
					continue
				}
				// Critical section: only one goroutine should be here at a time
				v := atomic.AddInt64(&counter, 1)
				if v != 1 {
					violations.Add(1)
				}
				atomic.AddInt64(&counter, -1)
				assert.NoError(t, h.Unlock())
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(0), violations.Load(), "mutual exclusion violated")
}

func TestConcurrentDifferentKeys(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	const numKeys = 10
	const numIterations = 100

	var wg sync.WaitGroup
	for i := 0; i < numKeys; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				h, err := kl.Acquire(context.Background(), key)
				if err != nil {
					continue
				}
				assert.NoError(t, h.Unlock())
			}
		}(fmt.Sprintf("key-%d", i))
	}

	wg.Wait()
	// All keys should be cleaned up
	assert.Empty(t, kl.Keys())
}

func TestMaxKeysConcurrent(t *testing.T) {
	const maxKeys = 10
	kl := newForTest(t, WithMaxKeys(maxKeys))
	defer func() { require.NoError(t, kl.Close()) }()

	var wg sync.WaitGroup
	var concurrentKeys atomic.Int64
	var exceeded atomic.Int64

	// 启动多个 goroutine 并发获取不同 key，验证 maxKeys 不被突破。
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			h, err := kl.TryAcquire(key)
			if err != nil {
				return // ErrLockOccupied, ErrMaxKeysExceeded, 或 ErrClosed
			}
			// 检测同时持有的 key 数是否超过上限
			cur := concurrentKeys.Add(1)
			if cur > int64(maxKeys) {
				exceeded.Add(1)
			}
			// 短暂持有锁，增加并发竞争
			time.Sleep(time.Millisecond)
			concurrentKeys.Add(-1)
			assert.NoError(t, h.Unlock())
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(0), exceeded.Load(), "concurrent keys should never exceed maxKeys")

	// 验证最终状态：所有锁已释放
	assert.Empty(t, kl.Keys())

	// 严格验证：同时持有不超过 maxKeys 个锁
	var handles []Handle
	for i := 0; i < maxKeys; i++ {
		h, err := kl.Acquire(context.Background(), fmt.Sprintf("strict-%d", i))
		require.NoError(t, err)
		handles = append(handles, h)
	}
	// 第 maxKeys+1 个必须失败
	_, err := kl.TryAcquire("strict-overflow")
	assert.ErrorIs(t, err, ErrMaxKeysExceeded)

	for _, h := range handles {
		assert.NoError(t, h.Unlock())
	}
}

func TestAcquireUnblockAfterRelease(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	acquired := make(chan struct{})
	started := make(chan struct{})
	go func() {
		close(started)
		h2, acqErr := kl.Acquire(context.Background(), "key1")
		if acqErr == nil {
			close(acquired)
			assert.NoError(t, h2.Unlock())
		}
	}()

	// 等待 goroutine 启动后释放锁
	<-started
	runtime.Gosched()
	require.NoError(t, h.Unlock())

	select {
	case <-acquired:
		// Success
	case <-time.After(time.Second):
		t.Fatal("second Acquire did not unblock after Unlock")
	}
}

func TestNewWithNilOption(t *testing.T) {
	// New(nil) 不应 panic。
	kl := newForTest(t, nil)
	require.NotNil(t, kl)
	require.NoError(t, kl.Close())
}

func TestWithMaxKeysZeroAndNegative(t *testing.T) {
	for _, n := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxKeys=%d", n), func(t *testing.T) {
			kl := newForTest(t, WithMaxKeys(n))
			defer func() { require.NoError(t, kl.Close()) }()

			handles := make([]Handle, 0, 20)
			for i := range 20 {
				h, err := kl.Acquire(context.Background(), fmt.Sprintf("key-%d", i))
				require.NoError(t, err, "WithMaxKeys(%d) should not limit keys", n)
				handles = append(handles, h)
			}
			for _, h := range handles {
				require.NoError(t, h.Unlock())
			}
		})
	}
}

func TestAcquireAlreadyCancelledContext(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := kl.Acquire(ctx, "key1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// 确保没有残留 entry
	assert.Empty(t, kl.Keys())
}

func TestAcquireClosedAndCancelledPriority(t *testing.T) {
	kl := newForTest(t)
	require.NoError(t, kl.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx 已取消

	// 同时满足 closed 和 ctx 取消时，ErrClosed 优先
	_, err := kl.Acquire(ctx, "key1")
	assert.ErrorIs(t, err, ErrClosed)
}

func TestTryAcquireAfterClose(t *testing.T) {
	kl := newForTest(t)
	require.NoError(t, kl.Close())

	h, err := kl.TryAcquire("key1")
	assert.Nil(t, h)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestKeysEmpty(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	keys := kl.Keys()
	assert.Empty(t, keys)
}

func TestLen(t *testing.T) {
	kl := newForTest(t)
	defer func() { require.NoError(t, kl.Close()) }()

	assert.Equal(t, 0, kl.Len())

	h1, err := kl.Acquire(context.Background(), "a")
	require.NoError(t, err)
	assert.Equal(t, 1, kl.Len())

	h2, err := kl.Acquire(context.Background(), "b")
	require.NoError(t, err)
	assert.Equal(t, 2, kl.Len())

	// 同一 key 再次获取不增加计数
	// (key "a" 已被持有，用 TryAcquire 验证不会产生新 key)
	_, err = kl.TryAcquire("a")
	assert.ErrorIs(t, err, ErrLockOccupied)
	assert.Equal(t, 2, kl.Len())

	require.NoError(t, h1.Unlock())
	assert.Equal(t, 1, kl.Len())

	require.NoError(t, h2.Unlock())
	assert.Equal(t, 0, kl.Len())
}

func TestCloseWakesWaiters(t *testing.T) {
	kl := newForTest(t)

	// 持有锁，让其他 goroutine 阻塞在 Acquire 上
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	const numWaiters = 10
	results := make(chan error, numWaiters)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(numWaiters)

	for range numWaiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			_, acqErr := kl.Acquire(context.Background(), "key1")
			results <- acqErr
		}()
	}

	// 等待所有 goroutine 启动，让出 CPU 使其进入阻塞
	ready.Wait()
	runtime.Gosched()

	// Close 应立即唤醒所有等待者
	require.NoError(t, kl.Close())

	// 等待所有 goroutine 完成（不应挂起）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功：所有等待者已被唤醒
	case <-time.After(time.Second):
		t.Fatal("Close did not wake all waiting Acquire goroutines")
	}

	close(results)
	for acqErr := range results {
		assert.ErrorIs(t, acqErr, ErrClosed)
	}

	require.NoError(t, h.Unlock())
}

func TestMultipleKeysConcurrentAcquireRelease(t *testing.T) {
	kl := newForTest(t, WithShardCount(4))
	defer func() { require.NoError(t, kl.Close()) }()

	const numKeys = 50
	const numIterations = 50

	var wg sync.WaitGroup
	for i := range numKeys {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", id)
			for range numIterations {
				h, err := kl.Acquire(context.Background(), key)
				if err != nil {
					continue
				}
				assert.NoError(t, h.Unlock())
			}
		}(i)
	}
	wg.Wait()
	assert.Empty(t, kl.Keys())
}

// TestTryAcquireCloseAfterSend 确定性覆盖 TryAcquire 的"获取成功后发现已关闭"回滚分支
// （keylock_impl.go: entry.ch 发送成功 → testHook 注入 Close → closed 检查 → 回滚）。
func TestTryAcquireCloseAfterSend(t *testing.T) {
	kl := newForTest(t)
	impl, ok := kl.(*keyLockImpl)
	require.True(t, ok)

	// 在 TryAcquire 发送成功后、closed 检查前注入 Close。
	impl.testHookAfterTryAcquireSend = func() {
		assert.NoError(t, kl.Close())
	}

	h, err := kl.TryAcquire("key1")
	assert.Nil(t, h)
	assert.ErrorIs(t, err, ErrClosed)

	// 验证引用计数和 key 正确清理
	assert.Equal(t, 0, kl.Len())
}

// TestTryAcquireDefaultCloseAfterReleaseRef 确定性覆盖 TryAcquire default 分支的
// "releaseRef 后发现已关闭"路径（keylock_impl.go: default → releaseRef → testHook 注入
// Close → closed 检查 → 返回 ErrClosed）。与 TestTryAcquireCloseAfterSend 配合，
// 完整覆盖 TryAcquire 的两条竞态分支。
func TestTryAcquireDefaultCloseAfterReleaseRef(t *testing.T) {
	kl := newForTest(t)
	impl, ok := kl.(*keyLockImpl)
	require.True(t, ok)

	// 持有锁，使 TryAcquire 走 default 分支。
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	// 在 default 分支 releaseRef 后、closed 检查前注入 Close。
	impl.testHookAfterDefaultReleaseRef = func() {
		assert.NoError(t, kl.Close())
	}

	h2, err := kl.TryAcquire("key1")
	assert.Nil(t, h2)
	assert.ErrorIs(t, err, ErrClosed)

	require.NoError(t, h.Unlock())
}

// TestTryAcquireCloseRace 压力测试 TryAcquire 与 Close 的并发安全性。
// 设计决策: 此测试为非确定性压力测试（配合 -race 检测数据竞争），
// 不断言特定分支命中——TryAcquire 的两条竞态分支分别由
// TestTryAcquireCloseAfterSend 和 TestTryAcquireDefaultCloseAfterReleaseRef
// 通过 hook 确定性覆盖。
func TestTryAcquireCloseRace(t *testing.T) {
	kl := newForTest(t)

	// 持有锁，使后续 TryAcquire 走 default 分支。
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	// TryAcquire 走 default 分支（锁被占用），然后检查 closed。
	// 在 releaseRef 之后、closed 检查之前 Close，使其返回 ErrClosed 而非 (nil, nil)。
	// 由于无法精确控制这个微小窗口，用循环 + 并发触发。
	const rounds = 5000
	for range rounds {
		kl2 := newForTest(t)

		// 持有锁制造 default 分支条件。
		holder, acqErr := kl2.Acquire(context.Background(), "x")
		require.NoError(t, acqErr)

		ready := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			close(ready)
			h2, tryErr := kl2.TryAcquire("x")
			if tryErr != nil {
				result <- tryErr
				return
			}
			assert.NoError(t, h2.Unlock()) // tryErr==nil 保证 h2 非 nil
			result <- nil
		}()

		<-ready
		runtime.Gosched()
		kl2.Close()     // 压力测试
		holder.Unlock() // 压力测试
		<-result
	}

	require.NoError(t, h.Unlock())
	require.NoError(t, kl.Close())
}

// TestCloseBarrier_Stress 验证 S1 修复：并发 Close + Acquire/TryAcquire 不会 panic、
// 死锁或数据竞争。结合 -race 标志运行以检测并发问题。
// 基本的"Close 后 Acquire 返回 ErrClosed"语义由 TestAcquireAfterClose 和
// TestTryAcquireAfterClose 覆盖。
func TestCloseBarrier_Stress(t *testing.T) {
	const rounds = 10_000

	for range rounds {
		kl := newForTest(t)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			h, err := kl.Acquire(context.Background(), "a")
			if err == nil {
				assert.NoError(t, h.Unlock())
			}
		}()

		go func() {
			defer wg.Done()
			h, err := kl.TryAcquire("b")
			if err == nil {
				assert.NoError(t, h.Unlock())
			}
		}()

		runtime.Gosched()
		// Close 可能返回 ErrClosed（若已被并发关闭），此处不断言。
		kl.Close() // 并发压力测试，Close 可重复调用

		wg.Wait()
	}
}
