package xkeylock

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkAcquireUnlock(b *testing.B) {
	kl := newForTest(b)
	defer kl.Close()

	ctx := context.Background()

	b.ReportAllocs()
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
	kl := newForTest(b)
	defer kl.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h, err := kl.TryAcquire("key")
		if err != nil {
			b.Fatal(err)
		}
		h.Unlock() // err==nil 保证 h 非 nil
	}
}

func BenchmarkAcquireUnlockParallel(b *testing.B) {
	// 预计算 key 数组，避免 fmt.Sprintf 在热路径上影响基准结果。
	const numKeys = 100
	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	for _, shards := range []int{1, 16, 32, 64} {
		b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
			kl := newForTest(b, WithShardCount(shards))
			defer kl.Close()

			ctx := context.Background()
			b.ReportAllocs()
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

func BenchmarkAcquireUnlockContended(b *testing.B) {
	kl := newForTest(b)
	defer kl.Close()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h, err := kl.Acquire(ctx, "contended-key")
			if err != nil {
				continue
			}
			h.Unlock()
		}
	})
}

func BenchmarkGetOrCreate(b *testing.B) {
	locker := newForTest(b)
	kl, ok := locker.(*keyLockImpl)
	if !ok {
		b.Fatal("unexpected Locker implementation")
	}
	defer kl.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		entry, err := kl.getOrCreate("key")
		if err != nil {
			b.Fatal(err)
		}
		kl.releaseRef("key", entry)
	}
}
