package xkeylock

import (
	"context"
	"testing"
)

func FuzzAcquireUnlock(f *testing.F) {
	f.Add("key1")
	f.Add("")
	f.Add("very-long-key-name-that-might-cause-issues-with-hashing")
	f.Add("key/with/slashes")
	f.Add("key with spaces")
	f.Add("中文key")

	f.Fuzz(func(t *testing.T, key string) {
		kl := New()
		defer kl.Close()

		h, err := kl.Acquire(context.Background(), key)
		if err != nil {
			t.Fatalf("Acquire failed for key %q: %v", key, err)
		}
		if h.Key() != key {
			t.Fatalf("Key mismatch: got %q, want %q", h.Key(), key)
		}
		if err := h.Unlock(); err != nil {
			t.Fatalf("Unlock failed for key %q: %v", key, err)
		}
	})
}

func FuzzTryAcquireUnlock(f *testing.F) {
	f.Add("key1")
	f.Add("")
	f.Add("a/b/c")
	f.Add("中文key")

	f.Fuzz(func(t *testing.T, key string) {
		kl := New()
		defer kl.Close()

		h, err := kl.TryAcquire(key)
		if err != nil {
			t.Fatalf("TryAcquire failed for key %q: %v", key, err)
		}
		if h == nil {
			t.Fatalf("TryAcquire returned nil handle for uncontended key %q", key)
		}
		if h.Key() != key {
			t.Fatalf("Key mismatch: got %q, want %q", h.Key(), key)
		}

		// 再次 TryAcquire 同一 key 应返回 nil（锁被占用）
		h2, err := kl.TryAcquire(key)
		if err != nil {
			t.Fatalf("second TryAcquire failed: %v", err)
		}
		if h2 != nil {
			h2.Unlock()
			t.Fatalf("second TryAcquire should return nil for held key %q", key)
		}

		if err := h.Unlock(); err != nil {
			t.Fatalf("Unlock failed for key %q: %v", key, err)
		}
	})
}
