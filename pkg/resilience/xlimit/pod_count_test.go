package xlimit

import (
	"context"
	"testing"
	"time"
)

func TestStaticPodCount(t *testing.T) {
	tests := []struct {
		name     string
		count    StaticPodCount
		expected int
	}{
		{"positive count", StaticPodCount(5), 5},
		{"zero defaults to 1", StaticPodCount(0), 1},
		{"negative defaults to 1", StaticPodCount(-1), 1},
		{"large count", StaticPodCount(100), 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			count, err := tc.count.GetPodCount(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, count)
			}
		})
	}
}

func TestEnvPodCount(t *testing.T) {
	const testEnvVar = "XLIMIT_TEST_POD_COUNT"

	t.Run("missing env var uses default", func(t *testing.T) {
		provider := NewEnvPodCount(testEnvVar+"_MISSING", 4)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 4 {
			t.Errorf("expected default 4, got %d", count)
		}
	})

	t.Run("env var with valid value", func(t *testing.T) {
		t.Setenv(testEnvVar, "8")

		provider := NewEnvPodCount(testEnvVar, 4)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 8 {
			t.Errorf("expected 8 from env, got %d", count)
		}
	})

	t.Run("env var with invalid value uses default", func(t *testing.T) {
		t.Setenv(testEnvVar+"_INVALID", "not-a-number")

		provider := NewEnvPodCount(testEnvVar+"_INVALID", 3)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 3 {
			t.Errorf("expected default 3, got %d", count)
		}
	})

	t.Run("env var with zero uses default", func(t *testing.T) {
		t.Setenv(testEnvVar+"_ZERO", "0")

		provider := NewEnvPodCount(testEnvVar+"_ZERO", 2)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected default 2, got %d", count)
		}
	})

	t.Run("env var with negative uses default", func(t *testing.T) {
		t.Setenv(testEnvVar+"_NEG", "-5")

		provider := NewEnvPodCount(testEnvVar+"_NEG", 6)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 6 {
			t.Errorf("expected default 6, got %d", count)
		}
	})

	t.Run("default count normalized", func(t *testing.T) {
		// 如果 defaultCount <= 0，应该被设置为 1
		provider := NewEnvPodCount(testEnvVar+"_MISSING2", 0)

		count, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected normalized default 1, got %d", count)
		}
	})
}

func TestEnvPodCount_WithCaching(t *testing.T) {
	const testEnvVar = "XLIMIT_TEST_CACHE_POD"

	t.Run("caching prevents repeated reads", func(t *testing.T) {
		t.Setenv(testEnvVar, "10")

		provider := NewEnvPodCount(testEnvVar, 1).WithCacheDuration(time.Second)

		// 首次读取
		count1, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count1 != 10 {
			t.Errorf("expected 10, got %d", count1)
		}

		// 修改环境变量
		t.Setenv(testEnvVar, "20")

		// 缓存期内应该返回旧值
		count2, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count2 != 10 {
			t.Errorf("expected cached 10, got %d", count2)
		}
	})

	t.Run("no caching reads every time", func(t *testing.T) {
		t.Setenv(testEnvVar+"_NOCACHE", "10")

		provider := NewEnvPodCount(testEnvVar+"_NOCACHE", 1) // 默认 CacheDuration 为 0

		// 首次读取
		count1, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count1 != 10 {
			t.Errorf("expected 10, got %d", count1)
		}

		// 修改环境变量
		t.Setenv(testEnvVar+"_NOCACHE", "20")

		// 无缓存时应该立即读取新值
		count2, err := provider.GetPodCount(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count2 != 20 {
			t.Errorf("expected new 20, got %d", count2)
		}
	})
}

func TestPodCountProvider_Interface(t *testing.T) {
	// 确保类型实现了接口
	var _ PodCountProvider = StaticPodCount(1)
	var _ PodCountProvider = &EnvPodCount{}
}
