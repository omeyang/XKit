package xplatform_test

import (
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
)

// sink 变量消费 benchmark 返回值，防止编译器死代码消除（DCE）导致结果失真。
var (
	sinkString string
	sinkBool   bool
)

// =============================================================================
// Config 方法 Benchmark
// =============================================================================

func BenchmarkConfig_Validate_Valid(b *testing.B) {
	cfg := xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConfig_Validate_Invalid(b *testing.B) {
	cfg := xplatform.Config{
		PlatformID: "",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err == nil {
			b.Fatal("expected validation error")
		}
	}
}

// =============================================================================
// 全局访问函数 Benchmark
// =============================================================================

func BenchmarkPlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkString = xplatform.PlatformID()
	}
}

func BenchmarkHasParent(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
		HasParent:  true,
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkBool = xplatform.HasParent()
	}
}

func BenchmarkUnclassRegionID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-benchmark",
		UnclassRegionID: "region-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkString = xplatform.UnclassRegionID()
	}
}

func BenchmarkIsInitialized(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkBool = xplatform.IsInitialized()
	}
}

func BenchmarkRequirePlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := xplatform.RequirePlatformID(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetConfig(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-benchmark",
		HasParent:       true,
		UnclassRegionID: "region-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := xplatform.GetConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 并发访问 Benchmark
// =============================================================================

func BenchmarkPlatformID_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sinkString = xplatform.PlatformID()
		}
	})
}

func BenchmarkHasParent_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
		HasParent:  true,
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sinkBool = xplatform.HasParent()
		}
	})
}

func BenchmarkIsInitialized_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sinkBool = xplatform.IsInitialized()
		}
	})
}

func BenchmarkGetConfig_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-parallel",
		HasParent:       true,
		UnclassRegionID: "region-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := xplatform.GetConfig(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
