package xcron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHook 用于测试的钩子实现
type testHook struct {
	beforeCalled atomic.Bool
	afterCalled  atomic.Bool
	beforeCtx    context.Context
	afterCtx     context.Context
	afterName    string
	afterDur     time.Duration
	afterErr     error
	mu           sync.Mutex
	// 用于验证执行顺序
	order     *[]string
	hookID    string
	contextID string // 用于注入到 context 的 ID
}

type ctxKey string

func (h *testHook) BeforeJob(ctx context.Context, name string) context.Context {
	h.beforeCalled.Store(true)
	h.mu.Lock()
	h.beforeCtx = ctx
	if h.order != nil {
		*h.order = append(*h.order, "before:"+h.hookID)
	}
	h.mu.Unlock()
	// 在 context 中注入标记
	if h.contextID != "" {
		ctx = context.WithValue(ctx, ctxKey(h.hookID), h.contextID)
	}
	return ctx
}

func (h *testHook) AfterJob(ctx context.Context, name string, duration time.Duration, err error) {
	h.afterCalled.Store(true)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.afterCtx = ctx
	h.afterName = name
	h.afterDur = duration
	h.afterErr = err
	if h.order != nil {
		*h.order = append(*h.order, "after:"+h.hookID)
	}
}

func TestHook_BasicExecution(t *testing.T) {
	hook := &testHook{hookID: "test"}
	opts := defaultJobOptions()
	opts.name = "test-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	assert.True(t, hook.beforeCalled.Load(), "BeforeJob should be called")
	assert.True(t, hook.afterCalled.Load(), "AfterJob should be called")
	assert.Equal(t, "test-job", hook.afterName)
	assert.NoError(t, hook.afterErr)
	assert.Greater(t, hook.afterDur, time.Duration(0))
}

func TestHook_WithError(t *testing.T) {
	hook := &testHook{hookID: "test"}
	opts := defaultJobOptions()
	opts.name = "error-job"
	opts.hooks = []Hook{hook}

	expectedErr := errors.New("job failed")
	job := JobFunc(func(ctx context.Context) error {
		return expectedErr
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	assert.True(t, hook.afterCalled.Load())
	assert.Equal(t, expectedErr, hook.afterErr)
}

func TestHook_ExecutionOrder(t *testing.T) {
	var order []string
	hook1 := &testHook{hookID: "hook1", order: &order}
	hook2 := &testHook{hookID: "hook2", order: &order}
	hook3 := &testHook{hookID: "hook3", order: &order}

	opts := defaultJobOptions()
	opts.name = "order-test-job"
	opts.hooks = []Hook{hook1, hook2, hook3}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	// BeforeJob: 正序执行 hook1 → hook2 → hook3
	// AfterJob: 逆序执行 hook3 → hook2 → hook1
	expected := []string{
		"before:hook1",
		"before:hook2",
		"before:hook3",
		"after:hook3",
		"after:hook2",
		"after:hook1",
	}
	assert.Equal(t, expected, order)
}

func TestHook_ContextPropagation(t *testing.T) {
	hook1 := &testHook{hookID: "hook1", contextID: "value1"}
	hook2 := &testHook{hookID: "hook2", contextID: "value2"}

	var receivedCtx context.Context
	opts := defaultJobOptions()
	opts.name = "ctx-test-job"
	opts.hooks = []Hook{hook1, hook2}

	job := JobFunc(func(ctx context.Context) error {
		receivedCtx = ctx
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	// 验证 job 收到了 hooks 注入的 context 值
	assert.Equal(t, "value1", receivedCtx.Value(ctxKey("hook1")))
	assert.Equal(t, "value2", receivedCtx.Value(ctxKey("hook2")))
}

func TestHook_NoHooks(t *testing.T) {
	opts := defaultJobOptions()
	opts.name = "no-hooks-job"
	// 不设置 hooks

	var executed bool
	job := JobFunc(func(ctx context.Context) error {
		executed = true
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	assert.True(t, executed)
}

func TestHook_WithLocker(t *testing.T) {
	hook := &testHook{hookID: "test"}
	locker := newMockLocker()

	opts := defaultJobOptions()
	opts.name = "locked-hook-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, locker, nil, nil, opts)
	wrapper.Run()

	assert.True(t, hook.beforeCalled.Load())
	assert.True(t, hook.afterCalled.Load())
}

func TestHook_LockNotAcquired(t *testing.T) {
	hook := &testHook{hookID: "test"}
	locker := newMockLocker()

	// 预先获取锁，使后续获取失败
	_, err := locker.TryLock(context.Background(), "skip-hook-job", time.Minute)
	require.NoError(t, err)

	opts := defaultJobOptions()
	opts.name = "skip-hook-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, locker, nil, nil, opts)
	wrapper.Run()

	// 锁未获取时，hooks 不应该被调用
	assert.False(t, hook.beforeCalled.Load(), "BeforeJob should not be called when lock not acquired")
	assert.False(t, hook.afterCalled.Load(), "AfterJob should not be called when lock not acquired")
}

func TestHookFunc_Adapter(t *testing.T) {
	var beforeCalled, afterCalled bool
	var receivedName string
	var receivedErr error

	hook := HookFunc{
		Before: func(ctx context.Context, name string) context.Context {
			beforeCalled = true
			return ctx
		},
		After: func(ctx context.Context, name string, d time.Duration, err error) {
			afterCalled = true
			receivedName = name
			receivedErr = err
		},
	}

	opts := defaultJobOptions()
	opts.name = "hookfunc-test-job"
	opts.hooks = []Hook{hook}

	expectedErr := errors.New("test error")
	job := JobFunc(func(ctx context.Context) error {
		return expectedErr
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	assert.True(t, beforeCalled)
	assert.True(t, afterCalled)
	assert.Equal(t, "hookfunc-test-job", receivedName)
	assert.Equal(t, expectedErr, receivedErr)
}

func TestHookFunc_NilFunctions(t *testing.T) {
	// 测试 HookFunc 的 Before/After 为 nil 时不会 panic
	hook := HookFunc{
		Before: nil,
		After:  nil,
	}

	ctx := context.Background()

	// 应该不会 panic
	newCtx := hook.BeforeJob(ctx, "test")
	assert.Equal(t, ctx, newCtx)

	hook.AfterJob(ctx, "test", time.Second, nil) // 不应 panic
}

func TestHookFunc_PartialFunctions(t *testing.T) {
	var afterCalled bool

	// 只设置 After，Before 为 nil
	hook := HookFunc{
		Before: nil,
		After: func(ctx context.Context, name string, d time.Duration, err error) {
			afterCalled = true
		},
	}

	opts := defaultJobOptions()
	opts.name = "partial-hook-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)
	wrapper.Run()

	assert.True(t, afterCalled)
}

func TestWithHook_Option(t *testing.T) {
	hook1 := &testHook{hookID: "hook1"}
	hook2 := &testHook{hookID: "hook2"}

	opts := defaultJobOptions()
	WithHook(hook1)(opts)
	WithHook(hook2)(opts)

	assert.Len(t, opts.hooks, 2)
	assert.Equal(t, hook1, opts.hooks[0])
	assert.Equal(t, hook2, opts.hooks[1])
}

func TestWithHook_NilIgnored(t *testing.T) {
	hook := &testHook{hookID: "hook1"}

	opts := defaultJobOptions()
	WithHook(nil)(opts) // nil 应被忽略
	WithHook(hook)(opts)
	WithHook(nil)(opts) // nil 应被忽略

	assert.Len(t, opts.hooks, 1)
	assert.Equal(t, hook, opts.hooks[0])
}

func TestWithHooks_Option(t *testing.T) {
	hook1 := &testHook{hookID: "hook1"}
	hook2 := &testHook{hookID: "hook2"}
	hook3 := &testHook{hookID: "hook3"}

	opts := defaultJobOptions()
	WithHooks(hook1, hook2, hook3)(opts)

	assert.Len(t, opts.hooks, 3)
}

func TestWithHooks_NilIgnored(t *testing.T) {
	hook1 := &testHook{hookID: "hook1"}
	hook2 := &testHook{hookID: "hook2"}

	opts := defaultJobOptions()
	WithHooks(nil, hook1, nil, hook2, nil)(opts)

	assert.Len(t, opts.hooks, 2)
}

func TestScheduler_WithHook(t *testing.T) {
	scheduler := New(WithSeconds())
	defer scheduler.Stop()

	hook := &testHook{hookID: "scheduler-hook"}
	executed := make(chan struct{}, 1)

	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		executed <- struct{}{}
		return nil
	}, WithName("hook-scheduler-test"), WithHook(hook))
	require.NoError(t, err)

	scheduler.Start()

	select {
	case <-executed:
		// 等待 hook 完成
		time.Sleep(50 * time.Millisecond)
		assert.True(t, hook.beforeCalled.Load())
		assert.True(t, hook.afterCalled.Load())
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for job execution")
	}
}

func TestHook_WithStats(t *testing.T) {
	hook := &testHook{hookID: "stats-hook"}
	stats := newStats()

	opts := defaultJobOptions()
	opts.name = "stats-hook-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, stats, opts)
	wrapper.Run()

	// 验证 hook 和 stats 都被调用
	assert.True(t, hook.afterCalled.Load())
	assert.Equal(t, int64(1), stats.TotalExecutions())
	assert.Equal(t, int64(1), stats.SuccessCount())
}

func TestHook_MultipleJobExecutions(t *testing.T) {
	var execCount atomic.Int32
	hook := HookFunc{
		Before: func(ctx context.Context, name string) context.Context {
			execCount.Add(1)
			return ctx
		},
		After: func(ctx context.Context, name string, d time.Duration, err error) {
			// 计数在 Before 中完成
		},
	}

	opts := defaultJobOptions()
	opts.name = "multi-exec-job"
	opts.hooks = []Hook{hook}

	job := JobFunc(func(ctx context.Context) error {
		return nil
	})

	wrapper := newJobWrapper(job, NoopLocker(), nil, nil, opts)

	// 执行多次
	for i := 0; i < 5; i++ {
		wrapper.Run()
	}

	assert.Equal(t, int32(5), execCount.Load())
}
