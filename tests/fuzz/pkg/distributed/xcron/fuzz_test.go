package xcron

import (
	"context"
	"testing"
	"time"
)

// FuzzJobFunc 模糊测试 JobFunc 适配器
func FuzzJobFunc(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, shouldSucceed bool) {
		var job JobFunc
		if shouldSucceed {
			job = func(_ context.Context) error { return nil }
		} else {
			job = func(_ context.Context) error { return errWrapper{"test error"} }
		}

		ctx := context.Background()
		err := job.Run(ctx)

		if shouldSucceed && err != nil {
			t.Error("Expected success")
		}
		if !shouldSucceed && err == nil {
			t.Error("Expected error")
		}
	})
}

// FuzzWithName 模糊测试 WithName 选项
func FuzzWithName(f *testing.F) {
	f.Add("")
	f.Add("my-job")
	f.Add("job with spaces")
	f.Add("job\x00null")
	f.Add("unicode任务🎯")

	f.Fuzz(func(t *testing.T, name string) {
		opts := defaultJobOptions()
		WithName(name)(opts)

		if opts.name != name {
			t.Errorf("Name mismatch: got %q, want %q", opts.name, name)
		}
	})
}

// FuzzWithTimeout 模糊测试 WithTimeout 选项
func FuzzWithTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1000))
	f.Add(int64(60000))
	f.Add(int64(-1000))
	f.Add(int64(3600000))

	f.Fuzz(func(t *testing.T, ms int64) {
		timeout := time.Duration(ms) * time.Millisecond
		opts := defaultJobOptions()
		originalTimeout := opts.timeout

		WithTimeout(timeout)(opts)

		// 只有正值才会被应用
		if timeout > 0 {
			if opts.timeout != timeout {
				t.Errorf("Timeout should be %v, got %v", timeout, opts.timeout)
			}
		} else {
			if opts.timeout != originalTimeout {
				t.Errorf("Timeout should remain unchanged for non-positive value")
			}
		}
	})
}

// FuzzWithLockTTL 模糊测试 WithLockTTL 选项
func FuzzWithLockTTL(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1000))
	f.Add(int64(300000))
	f.Add(int64(-1000))

	f.Fuzz(func(t *testing.T, ms int64) {
		ttl := time.Duration(ms) * time.Millisecond
		opts := defaultJobOptions()
		originalTTL := opts.lockTTL

		WithLockTTL(ttl)(opts)

		if ttl > 0 {
			// 考虑最小 TTL 强制（MinLockTTL = 3s）
			expectedTTL := max(ttl, MinLockTTL)
			if opts.lockTTL != expectedTTL {
				t.Errorf("LockTTL should be %v, got %v", expectedTTL, opts.lockTTL)
			}
		} else if opts.lockTTL != originalTTL {
			t.Errorf("LockTTL should remain unchanged for non-positive value")
		}
	})
}

// FuzzNoopLocker 模糊测试 NoopLocker
func FuzzNoopLocker(f *testing.F) {
	f.Add("lock-key", int64(60000))
	f.Add("", int64(0))
	f.Add("key\x00null", int64(-1000))
	f.Add("unicode锁🔒", int64(300000))

	f.Fuzz(func(t *testing.T, key string, ttlMs int64) {
		locker := NoopLocker()
		ctx := context.Background()
		ttl := time.Duration(ttlMs) * time.Millisecond

		// TryLock 总是成功（返回非 nil 的 LockHandle）
		handle, err := locker.TryLock(ctx, key, ttl)
		if err != nil {
			t.Errorf("TryLock should not error: %v", err)
		}
		if handle == nil {
			t.Error("TryLock should always return a handle")
		}

		// Unlock 总是成功
		if err := handle.Unlock(ctx); err != nil {
			t.Errorf("Unlock should not error: %v", err)
		}

		// 再次获取锁用于测试 Renew
		handle, _ = locker.TryLock(ctx, key, ttl)
		// Renew 总是成功
		if err := handle.Renew(ctx, ttl); err != nil {
			t.Errorf("Renew should not error: %v", err)
		}
	})
}

// FuzzNewScheduler 模糊测试调度器创建
func FuzzNewScheduler(f *testing.F) {
	f.Add(true, true)
	f.Add(false, true)
	f.Add(true, false)
	f.Add(false, false)

	f.Fuzz(func(t *testing.T, useLocker, useSeconds bool) {
		var opts []SchedulerOption

		if useLocker {
			opts = append(opts, WithLocker(NoopLocker()))
		}
		if useSeconds {
			opts = append(opts, WithSeconds())
		}

		scheduler := New(opts...)
		if scheduler == nil {
			t.Error("Scheduler should not be nil")
		}

		// 验证可以获取 Cron 实例
		if scheduler.Cron() == nil {
			t.Error("Cron should not be nil")
		}
	})
}

// FuzzAddFunc 模糊测试添加任务
func FuzzAddFunc(f *testing.F) {
	// 使用有效的 cron 表达式
	f.Add("@every 1m", "job1")
	f.Add("@hourly", "job2")
	f.Add("@daily", "job3")
	f.Add("0 * * * *", "job4")

	f.Fuzz(func(t *testing.T, spec, name string) {
		scheduler := New()
		job := func(_ context.Context) error { return nil }

		// 尝试添加任务
		id, err := scheduler.AddFunc(spec, job, WithName(name))

		// 无效的 cron 表达式会返回错误
		if err != nil {
			// 这是预期行为，无效表达式应该返回错误
			return
		}

		// 有效表达式应该返回有效 ID
		if id == 0 {
			t.Log("Got zero ID, might be valid for some implementations")
		}

		// 可以移除任务
		scheduler.Remove(id)
	})
}

// errWrapper 用于模糊测试的错误包装
type errWrapper struct {
	msg string
}

func (e errWrapper) Error() string {
	return e.msg
}
