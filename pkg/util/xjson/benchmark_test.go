package xjson

import "testing"

// benchSink 防止编译器优化掉基准测试中的函数调用结果。
var benchSink string

func BenchmarkPretty(b *testing.B) {
	type S struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	v := S{Name: "test", Value: 42}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchSink = Pretty(v)
	}
}

func BenchmarkPrettyMap(b *testing.B) {
	v := map[string]any{
		"name":  "test",
		"value": 42,
		"nested": map[string]string{
			"key": "val",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchSink = Pretty(v)
	}
}

func BenchmarkPrettyLargeObject(b *testing.B) {
	type Item struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	items := make([]Item, 100)
	for i := range items {
		items[i] = Item{ID: i, Name: "item", Value: "data"}
	}
	v := map[string]any{
		"items":   items,
		"total":   len(items),
		"version": "1.0",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchSink = Pretty(v)
	}
}

func BenchmarkPrettyError(b *testing.B) {
	v := make(chan int)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchSink = Pretty(v)
	}
}

func BenchmarkPrettyE(b *testing.B) {
	type S struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	v := S{Name: "test", Value: 42}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchSink, _ = PrettyE(v)
	}
}
