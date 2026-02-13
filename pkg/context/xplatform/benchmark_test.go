package xplatform_test

import (
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
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
		_ = xplatform.PlatformID()
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
		_ = xplatform.HasParent()
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
		_ = xplatform.UnclassRegionID()
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
		_ = xplatform.IsInitialized()
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
			_ = xplatform.PlatformID()
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
			_ = xplatform.HasParent()
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
			_ = xplatform.IsInitialized()
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
