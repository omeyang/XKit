package xkeylock

import (
	"context"
	"fmt"
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

func TestAcquireNilContext(t *testing.T) {
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	assert.PanicsWithValue(t, "xkeylock: nil Context", func() {
		kl.Acquire(nil, "key1") //nolint:errcheck,staticcheck // 测试 nil ctx panic 行为
	})
}

func TestAcquireAndUnlock(t *testing.T) {
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)
	require.NotNil(t, h)
	assert.Equal(t, "key1", h.Key())

	assert.NoError(t, h.Unlock())
}

func TestUnlockIdempotent(t *testing.T) {
	kl := New()
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
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	// First acquire succeeds
	h1, err := kl.TryAcquire("key1")
	require.NoError(t, err)
	require.NotNil(t, h1)

	// Second acquire fails (lock held)
	h2, err := kl.TryAcquire("key1")
	assert.NoError(t, err)
	assert.Nil(t, h2) // nil handle, nil error = lock occupied

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
	kl := New()
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
	kl := New()
	require.NoError(t, kl.Close())

	_, err := kl.Acquire(context.Background(), "key1")
	assert.ErrorIs(t, err, ErrClosed)

	_, err = kl.TryAcquire("key1")
	assert.ErrorIs(t, err, ErrClosed)
}

func TestCloseIdempotent(t *testing.T) {
	kl := New()
	assert.NoError(t, kl.Close())
	assert.ErrorIs(t, kl.Close(), ErrClosed)
}

func TestCloseDoesNotAffectHeldLocks(t *testing.T) {
	kl := New()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	require.NoError(t, kl.Close())

	// Unlock still works
	assert.NoError(t, h.Unlock())
}

func TestKeys(t *testing.T) {
	kl := New()
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
	kl := New(WithMaxKeys(2))
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
	// Valid power of 2
	kl := New(WithShardCount(64))
	impl, ok := kl.(*keyLockImpl)
	require.True(t, ok)
	assert.Equal(t, 64, len(impl.shards))
	require.NoError(t, kl.Close())

	// Invalid (not power of 2) — panics
	assert.PanicsWithValue(t, "xkeylock: shard count must be a positive power of 2", func() {
		New(WithShardCount(3))
	})

	// Zero — panics
	assert.PanicsWithValue(t, "xkeylock: shard count must be a positive power of 2", func() {
		New(WithShardCount(0))
	})

	// 注意：负数无需测试，WithShardCount 参数为 uint 类型，负数在编译期即报错。
}

func TestConcurrentMutualExclusion(t *testing.T) {
	kl := New()
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
	kl := New()
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
		}(string(rune('A' + i)))
	}

	wg.Wait()
	// All keys should be cleaned up
	assert.Empty(t, kl.Keys())
}

func TestMaxKeysConcurrent(t *testing.T) {
	const maxKeys = 10
	kl := New(WithMaxKeys(maxKeys))
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
				return
			}
			if h == nil {
				return
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
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	acquired := make(chan struct{})
	go func() {
		h2, acqErr := kl.Acquire(context.Background(), "key1")
		if acqErr == nil {
			close(acquired)
			assert.NoError(t, h2.Unlock())
		}
	}()

	// Release the lock
	time.Sleep(10 * time.Millisecond)
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
	kl := New(nil)
	require.NotNil(t, kl)
	require.NoError(t, kl.Close())
}

func TestWithMaxKeysZeroAndNegative(t *testing.T) {
	// WithMaxKeys(0) 表示不限制
	kl := New(WithMaxKeys(0))
	defer func() { require.NoError(t, kl.Close()) }()

	handles := make([]Handle, 0, 20)
	for i := range 20 {
		h, err := kl.Acquire(context.Background(), fmt.Sprintf("key-%d", i))
		require.NoError(t, err, "WithMaxKeys(0) should not limit keys")
		handles = append(handles, h)
	}
	for _, h := range handles {
		require.NoError(t, h.Unlock())
	}
}

func TestWithMaxKeysNegative(t *testing.T) {
	// WithMaxKeys(-1) 归一化为 0，即不限制
	kl := New(WithMaxKeys(-1))
	defer func() { require.NoError(t, kl.Close()) }()

	handles := make([]Handle, 0, 20)
	for i := range 20 {
		h, err := kl.Acquire(context.Background(), fmt.Sprintf("key-%d", i))
		require.NoError(t, err, "WithMaxKeys(-1) should not limit keys")
		handles = append(handles, h)
	}
	for _, h := range handles {
		require.NoError(t, h.Unlock())
	}
}

func TestAcquireAlreadyCancelledContext(t *testing.T) {
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := kl.Acquire(ctx, "key1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// 确保没有残留 entry
	assert.Empty(t, kl.Keys())
}

func TestTryAcquireAfterClose(t *testing.T) {
	kl := New()
	require.NoError(t, kl.Close())

	h, err := kl.TryAcquire("key1")
	assert.Nil(t, h)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestKeysEmpty(t *testing.T) {
	kl := New()
	defer func() { require.NoError(t, kl.Close()) }()

	keys := kl.Keys()
	assert.Empty(t, keys)
}

func TestLen(t *testing.T) {
	kl := New()
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
	h3, err := kl.TryAcquire("a")
	require.NoError(t, err)
	assert.Nil(t, h3) // lock held
	assert.Equal(t, 2, kl.Len())

	require.NoError(t, h1.Unlock())
	assert.Equal(t, 1, kl.Len())

	require.NoError(t, h2.Unlock())
	assert.Equal(t, 0, kl.Len())
}

func TestConcurrentAcquireAndClose(t *testing.T) {
	kl := New()

	// 持有一个锁，让其他 goroutine 阻塞在 Acquire 上
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	const numWaiters = 10
	errs := make(chan error, numWaiters)
	var wg sync.WaitGroup

	for range numWaiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 使用 Background（无超时），验证 Close 能唤醒等待者
			_, acqErr := kl.Acquire(context.Background(), "key1")
			if acqErr != nil {
				errs <- acqErr
			}
		}()
	}

	// 等待 goroutine 开始阻塞
	time.Sleep(20 * time.Millisecond)

	// 关闭 KeyLock，所有阻塞的 goroutine 应被立即唤醒
	require.NoError(t, kl.Close())
	require.NoError(t, h.Unlock())

	wg.Wait()
	close(errs)

	// 所有等待者应收到 ErrClosed
	for acqErr := range errs {
		assert.ErrorIs(t, acqErr, ErrClosed)
	}
}

func TestCloseWakesWaiters(t *testing.T) {
	kl := New()

	// 持有锁
	h, err := kl.Acquire(context.Background(), "key1")
	require.NoError(t, err)

	const numWaiters = 5
	results := make(chan error, numWaiters)
	var wg sync.WaitGroup

	for range numWaiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// context.Background() 无超时，完全依赖 Close 唤醒
			_, acqErr := kl.Acquire(context.Background(), "key1")
			results <- acqErr
		}()
	}

	// 等待所有 goroutine 进入阻塞
	time.Sleep(20 * time.Millisecond)

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
	kl := New(WithShardCount(4))
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
