package xplatform_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
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
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		if !xplatform.IsInitialized() {
			t.Error("IsInitialized() = false, want true")
		}
		if xplatform.PlatformID() != "platform-001" {
			t.Errorf("PlatformID() = %q, want %q", xplatform.PlatformID(), "platform-001")
		}
		if !xplatform.HasParent() {
			t.Error("HasParent() = false, want true")
		}
		if xplatform.UnclassRegionID() != "region-001" {
			t.Errorf("UnclassRegionID() = %q, want %q", xplatform.UnclassRegionID(), "region-001")
		}
	})

	t.Run("缺少PlatformID返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{})
		if !errors.Is(err, xplatform.ErrMissingPlatformID) {
			t.Errorf("Init() error = %v, want %v", err, xplatform.ErrMissingPlatformID)
		}
		if xplatform.IsInitialized() {
			t.Error("IsInitialized() = true after failed init, want false")
		}
	})

	t.Run("重复初始化返回错误", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
		if err != nil {
			t.Fatalf("first Init() error = %v", err)
		}

		err = xplatform.Init(xplatform.Config{PlatformID: "platform-002"})
		if !errors.Is(err, xplatform.ErrAlreadyInitialized) {
			t.Errorf("second Init() error = %v, want %v", err, xplatform.ErrAlreadyInitialized)
		}

		// 验证原值未被覆盖
		if xplatform.PlatformID() != "platform-001" {
			t.Errorf("PlatformID() = %q, want %q (should not be overwritten)", xplatform.PlatformID(), "platform-001")
		}
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

	_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: true})
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

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
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

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: true})
		if !xplatform.HasParent() {
			t.Error("HasParent() = false, want true")
		}
	})

	t.Run("初始化为false返回false", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001", HasParent: false})
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

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
		if got := xplatform.UnclassRegionID(); got != "" {
			t.Errorf("UnclassRegionID() = %q, want empty", got)
		}
	})

	t.Run("设置后返回正确值", func(t *testing.T) {
		xplatform.Reset()
		t.Cleanup(xplatform.Reset)

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001", UnclassRegionID: "region-001"})
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

	_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
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

		_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001"})
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
		_ = xplatform.Init(expected)

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

	_ = xplatform.Init(xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})

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
// 示例测试
// =============================================================================

func ExampleInit() {
	xplatform.Reset()
	defer xplatform.Reset()

	// 从 AUTH 服务获取平台信息后初始化
	_ = xplatform.Init(xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})

	fmt.Println("PlatformID:", xplatform.PlatformID())
	fmt.Println("HasParent:", xplatform.HasParent())
	fmt.Println("UnclassRegionID:", xplatform.UnclassRegionID())
	// Output:
	// PlatformID: platform-001
	// HasParent: true
	// UnclassRegionID: region-001
}

func ExampleRequirePlatformID() {
	xplatform.Reset()
	defer xplatform.Reset()

	_ = xplatform.Init(xplatform.Config{PlatformID: "platform-001"})

	pid, err := xplatform.RequirePlatformID()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("PlatformID:", pid)
	// Output:
	// PlatformID: platform-001
}

func ExampleGetConfig() {
	xplatform.Reset()
	defer xplatform.Reset()

	_ = xplatform.Init(xplatform.Config{
		PlatformID: "platform-001",
		HasParent:  true,
	})

	cfg, err := xplatform.GetConfig()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("PlatformID:", cfg.PlatformID)
	fmt.Println("HasParent:", cfg.HasParent)
	// Output:
	// PlatformID: platform-001
	// HasParent: true
}
