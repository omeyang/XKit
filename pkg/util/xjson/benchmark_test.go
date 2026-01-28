package xjson

import "testing"

func BenchmarkPretty(b *testing.B) {
	type S struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	v := S{Name: "test", Value: 42}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Pretty(v)
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
		_ = Pretty(v)
	}
}

func BenchmarkPrettyError(b *testing.B) {
	v := make(chan int)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Pretty(v)
	}
}
