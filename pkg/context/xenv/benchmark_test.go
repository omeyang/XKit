package xenv_test

import (
	"testing"

	"github.com/omeyang/xkit/pkg/context/xenv"
)

// =============================================================================
// DeployType 方法 Benchmark
// =============================================================================

func BenchmarkDeployType_String(b *testing.B) {
	dt := xenv.DeployLocal
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dt.String()
	}
}

func BenchmarkDeployType_IsLocal(b *testing.B) {
	dt := xenv.DeployLocal
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dt.IsLocal()
	}
}

func BenchmarkDeployType_IsSaaS(b *testing.B) {
	dt := xenv.DeploySaaS
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dt.IsSaaS()
	}
}

func BenchmarkDeployType_IsValid(b *testing.B) {
	dt := xenv.DeployLocal
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dt.IsValid()
	}
}

// =============================================================================
// Parse 函数 Benchmark
// =============================================================================

func BenchmarkParse_LOCAL(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = xenv.Parse("LOCAL")
	}
}

func BenchmarkParse_local(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = xenv.Parse("local")
	}
}

func BenchmarkParse_SAAS(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = xenv.Parse("SAAS")
	}
}

func BenchmarkParse_Invalid(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = xenv.Parse("invalid")
	}
}

// =============================================================================
// 全局访问函数 Benchmark
// =============================================================================

func BenchmarkType(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xenv.Type()
	}
}

func BenchmarkIsLocal(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xenv.IsLocal()
	}
}

func BenchmarkIsSaaS(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeploySaaS); err != nil {
		b.Fatalf("InitWith(DeploySaaS) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xenv.IsSaaS()
	}
}

func BenchmarkIsInitialized(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xenv.IsInitialized()
	}
}

func BenchmarkRequireType(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = xenv.RequireType()
	}
}

func BenchmarkType_NotInitialized(b *testing.B) {
	xenv.Reset()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xenv.Type()
	}
}

func BenchmarkRequireType_NotInitialized(b *testing.B) {
	xenv.Reset()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = xenv.RequireType()
	}
}

func BenchmarkParse_Empty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = xenv.Parse("")
	}
}

// =============================================================================
// 并发访问 Benchmark
// =============================================================================

func BenchmarkType_Parallel(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xenv.Type()
		}
	})
}

func BenchmarkIsLocal_Parallel(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xenv.IsLocal()
		}
	})
}

func BenchmarkIsInitialized_Parallel(b *testing.B) {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		b.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	b.Cleanup(xenv.Reset)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xenv.IsInitialized()
		}
	})
}
