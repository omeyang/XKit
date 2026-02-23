package xplatform_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Config 测试
// =============================================================================

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config xplatform.Config
		err    error
	}{
		{
			name:   "有效配置",
			config: xplatform.Config{PlatformID: "platform-001"},
			err:    nil,
		},
		{
			name:   "完整配置",
			config: xplatform.Config{PlatformID: "platform-001", HasParent: true, UnclassRegionID: "region-001"},
			err:    nil,
		},
		{
			name:   "缺少PlatformID",
			config: xplatform.Config{},
			err:    xplatform.ErrMissingPlatformID,
		},
		{
			name:   "PlatformID为空字符串",
			config: xplatform.Config{PlatformID: ""},
			err:    xplatform.ErrMissingPlatformID,
		},
		{
			name:   "PlatformID为纯空白",
			config: xplatform.Config{PlatformID: "   "},
			err:    xplatform.ErrMissingPlatformID,
		},
		{
			name:   "PlatformID包含空格",
			config: xplatform.Config{PlatformID: "platform 001"},
			err:    xplatform.ErrInvalidPlatformID,
		},
		{
			name:   "PlatformID包含制表符",
			config: xplatform.Config{PlatformID: "platform\t001"},
			err:    xplatform.ErrInvalidPlatformID,
		},
		{
			name:   "PlatformID包含控制字符NUL",
			config: xplatform.Config{PlatformID: "platform\x00001"},
			err:    xplatform.ErrInvalidPlatformID,
		},
		{
			name:   "PlatformID包含控制字符ESC",
			config: xplatform.Config{PlatformID: "platform\x1b001"},
			err:    xplatform.ErrInvalidPlatformID,
		},
		{
			name:   "PlatformID超过最大长度",
			config: xplatform.Config{PlatformID: strings.Repeat("a", 129)},
			err:    xplatform.ErrInvalidPlatformID,
		},
		{
			name:   "PlatformID恰好最大长度",
			config: xplatform.Config{PlatformID: strings.Repeat("a", 128)},
			err:    nil,
		},
		// UnclassRegionID 校验
		{
			name:   "UnclassRegionID为空（可选字段）",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: ""},
			err:    nil,
		},
		{
			name:   "UnclassRegionID有效",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region-001"},
			err:    nil,
		},
		{
			name:   "UnclassRegionID包含空格",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region 001"},
			err:    xplatform.ErrInvalidUnclassRegionID,
		},
		{
			name:   "UnclassRegionID包含制表符",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region\t001"},
			err:    xplatform.ErrInvalidUnclassRegionID,
		},
		{
			name:   "UnclassRegionID包含换行符",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region\n001"},
			err:    xplatform.ErrInvalidUnclassRegionID,
		},
		{
			name:   "UnclassRegionID包含控制字符NUL",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region\x00001"},
			err:    xplatform.ErrInvalidUnclassRegionID,
		},
		{
			name:   "UnclassRegionID超过最大长度",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: strings.Repeat("r", 129)},
			err:    xplatform.ErrInvalidUnclassRegionID,
		},
		{
			name:   "UnclassRegionID恰好最大长度",
			config: xplatform.Config{PlatformID: "platform-001", UnclassRegionID: strings.Repeat("r", 128)},
			err:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("Config.Validate() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Errorf("Config.Validate() error = %v, want nil", err)
			}
		})
	}
}

// =============================================================================
// Init/MustInit/Reset 测试
// =============================================================================

func TestInit(t *testing.T) {
	t.Run("正常初始化", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{
			PlatformID:      "platform-001",
			HasParent:       true,
			UnclassRegionID: "region-001",
		})
		require.NoError(t, err, "Init() should succeed")
		assert.True(t, xplatform.IsInitialized(), "IsInitialized() should be true")
		assert.Equal(t, "platform-001", xplatform.PlatformID())
		assert.True(t, xplatform.HasParent(), "HasParent() should be true")
		assert.Equal(t, "region-001", xplatform.UnclassRegionID())
	})

	t.Run("缺少PlatformID返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{})
		assert.ErrorIs(t, err, xplatform.ErrMissingPlatformID)
		assert.False(t, xplatform.IsInitialized(), "IsInitialized() should be false after failed init")
	})

	t.Run("重复初始化返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
		require.NoError(t, err, "first Init() should succeed")

		err = xplatform.Init(xplatform.Config{PlatformID: "platform-002"})
		assert.ErrorIs(t, err, xplatform.ErrAlreadyInitialized)

		// 验证原值未被覆盖
		assert.Equal(t, "platform-001", xplatform.PlatformID(), "PlatformID should not be overwritten")
	})

	t.Run("已初始化后传无效配置仍返回ErrAlreadyInitialized", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
		require.NoError(t, err, "first Init() should succeed")

		// 错误优先级：ErrAlreadyInitialized > 配置校验错误
		err = xplatform.Init(xplatform.Config{})
		assert.ErrorIs(t, err, xplatform.ErrAlreadyInitialized)
	})
}

func TestMustInit(t *testing.T) {
	t.Run("成功初始化不panic", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustInit() panicked: %v", r)
			}
		}()
		xplatform.MustInit(xplatform.Config{PlatformID: "platform-001"})
		if xplatform.PlatformID() != "platform-001" {
			t.Errorf("PlatformID() = %q, want %q", xplatform.PlatformID(), "platform-001")
		}
	})

	t.Run("配置无效时panic", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustInit() did not panic")
			}
		}()
		xplatform.MustInit(xplatform.Config{})
	})
}

func TestReset(t *testing.T) {
	xplatform.Reset()
	t.Cleanup(xplatform.Reset)

	if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: true}); err != nil {
		t.Fatal("Init() failed:", err)
	}
	if !xplatform.IsInitialized() {
		t.Fatal("IsInitialized() = false after Init")
	}

	xplatform.Reset()
	if xplatform.IsInitialized() {
		t.Error("IsInitialized() = true after Reset, want false")
	}
	if xplatform.PlatformID() != "" {
		t.Errorf("PlatformID() = %q after Reset, want empty", xplatform.PlatformID())
	}
	if xplatform.HasParent() {
		t.Error("HasParent() = true after Reset, want false")
	}
}

// =============================================================================
// 全局访问函数测试
// =============================================================================

func TestPlatformID(t *testing.T) {
	t.Run("未初始化返回空字符串", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if got := xplatform.PlatformID(); got != "" {
			t.Errorf("PlatformID() = %q, want empty", got)
		}
	})

	t.Run("已初始化返回正确值", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		if got := xplatform.PlatformID(); got != "platform-001" {
			t.Errorf("PlatformID() = %q, want %q", got, "platform-001")
		}
	})
}

func TestHasParent(t *testing.T) {
	t.Run("未初始化返回false", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if xplatform.HasParent() {
			t.Error("HasParent() = true, want false (not initialized)")
		}
	})

	t.Run("初始化为true返回true", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: true}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		if !xplatform.HasParent() {
			t.Error("HasParent() = false, want true")
		}
	})

	t.Run("初始化为false返回false", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: false}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		if xplatform.HasParent() {
			t.Error("HasParent() = true, want false")
		}
	})
}

func TestUnclassRegionID(t *testing.T) {
	t.Run("未初始化返回空字符串", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if got := xplatform.UnclassRegionID(); got != "" {
			t.Errorf("UnclassRegionID() = %q, want empty", got)
		}
	})

	t.Run("未设置返回空字符串", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		if got := xplatform.UnclassRegionID(); got != "" {
			t.Errorf("UnclassRegionID() = %q, want empty", got)
		}
	})

	t.Run("设置后返回正确值", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region-001"}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		if got := xplatform.UnclassRegionID(); got != "region-001" {
			t.Errorf("UnclassRegionID() = %q, want %q", got, "region-001")
		}
	})
}

func TestIsInitialized(t *testing.T) {
	xplatform.Reset()
	t.Cleanup(xplatform.Reset)

	if xplatform.IsInitialized() {
		t.Error("IsInitialized() = true, want false")
	}

	if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"}); err != nil {
		t.Fatal("Init() failed:", err)
	}
	if !xplatform.IsInitialized() {
		t.Error("IsInitialized() = false, want true")
	}
}

// =============================================================================
// RequirePlatformID/GetConfig 测试
// =============================================================================

func TestRequirePlatformID(t *testing.T) {
	t.Run("未初始化返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		_, err := xplatform.RequirePlatformID()
		if !errors.Is(err, xplatform.ErrNotInitialized) {
			t.Errorf("RequirePlatformID() error = %v, want %v", err, xplatform.ErrNotInitialized)
		}
	})

	t.Run("已初始化返回正确值", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"}); err != nil {
			t.Fatal("Init() failed:", err)
		}
		got, err := xplatform.RequirePlatformID()
		if err != nil {
			t.Fatalf("RequirePlatformID() error = %v", err)
		}
		if got != "platform-001" {
			t.Errorf("RequirePlatformID() = %q, want %q", got, "platform-001")
		}
	})
}

func TestGetConfig(t *testing.T) {
	t.Run("未初始化返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		_, err := xplatform.GetConfig()
		if !errors.Is(err, xplatform.ErrNotInitialized) {
			t.Errorf("GetConfig() error = %v, want %v", err, xplatform.ErrNotInitialized)
		}
	})

	t.Run("已初始化返回配置副本", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		expected := xplatform.Config{
			PlatformID:      "platform-001",
			HasParent:       true,
			UnclassRegionID: "region-001",
		}
		if err := xplatform.Init(expected); err != nil {
			t.Fatal("Init() failed:", err)
		}

		got, err := xplatform.GetConfig()
		if err != nil {
			t.Fatalf("GetConfig() error = %v", err)
		}
		if got.PlatformID != expected.PlatformID {
			t.Errorf("GetConfig().PlatformID = %q, want %q", got.PlatformID, expected.PlatformID)
		}
		if got.HasParent != expected.HasParent {
			t.Errorf("GetConfig().HasParent = %v, want %v", got.HasParent, expected.HasParent)
		}
		if got.UnclassRegionID != expected.UnclassRegionID {
			t.Errorf("GetConfig().UnclassRegionID = %q, want %q", got.UnclassRegionID, expected.UnclassRegionID)
		}
	})
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	xplatform.Reset()
	t.Cleanup(xplatform.Reset)

	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	}); err != nil {
		t.Fatal("Init() failed:", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = xplatform.PlatformID()
			_ = xplatform.HasParent()
			_ = xplatform.UnclassRegionID()
			_ = xplatform.IsInitialized()
			_, _ = xplatform.RequirePlatformID()
			_, _ = xplatform.GetConfig()
		}()
	}

	wg.Wait()
}

// =============================================================================
// 并发初始化测试
// =============================================================================

func TestConcurrentInit(t *testing.T) {
	xplatform.Reset()
	t.Cleanup(xplatform.Reset)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make(chan error, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			errs <- xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
		}()
	}

	wg.Wait()
	close(errs)

	var successes, alreadyInits int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, xplatform.ErrAlreadyInitialized):
			alreadyInits++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d", successes)
	}
	if alreadyInits != goroutines-1 {
		t.Errorf("expected %d ErrAlreadyInitialized, got %d", goroutines-1, alreadyInits)
	}
}
