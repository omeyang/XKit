package xkeylock

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkAcquireUnlock(b *testing.B) {
	kl := New()
	defer kl.Close()

	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		h, err := kl.Acquire(ctx, "key")
		if err != nil {
			b.Fatal(err)
		}
		h.Unlock()
	}
}

func BenchmarkTryAcquireUnlock(b *testing.B) {
	kl := New()
	defer kl.Close()

	b.ResetTimer()
	for b.Loop() {
		h, err := kl.TryAcquire("key")
		if err != nil {
			b.Fatal(err)
		}
		if h != nil {
			h.Unlock()
		}
	}
}

func BenchmarkAcquireUnlockParallel(b *testing.B) {
	// 预计算 key 数组，避免 fmt.Sprintf 在热路径上影响基准结果。
	const numKeys = 100
	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	for _, shards := range []uint{1, 16, 32, 64} {
		b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
			kl := New(WithShardCount(shards))
			defer kl.Close()

			ctx := context.Background()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := keys[i%numKeys]
					h, err := kl.Acquire(ctx, key)
					if err != nil {
						continue
					}
					h.Unlock()
					i++
				}
			})
		})
	}
}

func BenchmarkGetOrCreate(b *testing.B) {
	kl := New().(*keyLockImpl)
	defer kl.Close()

	b.ResetTimer()
	for b.Loop() {
		entry, err := kl.getOrCreate("key")
		if err != nil {
			b.Fatal(err)
		}
		kl.releaseRef("key", entry)
	}
}
