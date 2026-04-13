package xetcd

import (
	"bytes"
	"sync"
	"testing"
)

func TestInformerStore_SetAndGet(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("v1"), 10)
	v, ok := s.Get("a")
	if !ok || !bytes.Equal(v, []byte("v1")) {
		t.Fatalf("get = %q ok=%v", v, ok)
	}
	if s.Rev() != 10 {
		t.Errorf("rev = %d", s.Rev())
	}
}

func TestInformerStore_GetNotFound(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	v, ok := s.Get("missing")
	if ok || v != nil {
		t.Errorf("get missing = %v, %v", v, ok)
	}
}

func TestInformerStore_GetReturnsCopy(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("original"), 1)
	v, _ := s.Get("a")
	v[0] = 'X'
	v2, _ := s.Get("a")
	if string(v2) != "original" {
		t.Errorf("mutation leaked: %q", v2)
	}
}

func TestInformerStore_Remove(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("v"), 1)
	s.RemoveForTest("a", 2)
	if _, ok := s.Get("a"); ok {
		t.Error("should be removed")
	}
	if s.Rev() != 2 {
		t.Errorf("rev = %d", s.Rev())
	}
}

func TestInformerStore_Replace(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("v"), 1)
	s.replace(map[string][]byte{"b": []byte("v2")}, 5)
	if _, ok := s.Get("a"); ok {
		t.Error("a should be gone after replace")
	}
	v, ok := s.Get("b")
	if !ok || string(v) != "v2" {
		t.Errorf("b = %q ok=%v", v, ok)
	}
	if s.Rev() != 5 {
		t.Errorf("rev = %d", s.Rev())
	}
}

func TestInformerStore_List(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("1"), 1)
	s.SetForTest("b", []byte("2"), 2)
	m := s.List()
	if len(m) != 2 {
		t.Errorf("len = %d", len(m))
	}
	// 改外部返回不应影响 Store
	m["a"][0] = 'X'
	v, _ := s.Get("a")
	if string(v) != "1" {
		t.Errorf("List should return copy, got %q", v)
	}
}

func TestInformerStore_Keys(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("1"), 1)
	s.SetForTest("b", []byte("2"), 2)
	keys := s.Keys()
	if len(keys) != 2 {
		t.Errorf("keys len = %d", len(keys))
	}
}

func TestInformerStore_Len(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	if s.Len() != 0 {
		t.Error("empty len != 0")
	}
	s.SetForTest("a", []byte("1"), 1)
	if s.Len() != 1 {
		t.Error("len != 1 after set")
	}
}

func TestInformerStore_RevMonotonic(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	s.SetForTest("a", []byte("v"), 10)
	s.SetForTest("b", []byte("v"), 5) // 低 rev 不应回退
	if s.Rev() != 10 {
		t.Errorf("rev should stay at 10, got %d", s.Rev())
	}
}

func TestInformerStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	s := NewInformerStore()
	const g = 16
	const iters = 200
	var wg sync.WaitGroup
	for i := 0; i < g; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				s.SetForTest("k", []byte("v"), int64(id*iters+j))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_, _ = s.Get("k")
				_ = s.Len()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkInformerStore_Set(b *testing.B) {
	s := NewInformerStore()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SetForTest("k", []byte("v"), int64(i))
	}
}

func BenchmarkInformerStore_Get(b *testing.B) {
	s := NewInformerStore()
	s.SetForTest("k", []byte("v"), 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get("k")
	}
}

func BenchmarkInformerStore_List(b *testing.B) {
	s := NewInformerStore()
	for i := 0; i < 100; i++ {
		s.SetForTest(string(rune('a'+i%26)), []byte("v"), int64(i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.List()
	}
}
