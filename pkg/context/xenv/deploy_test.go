package xenv_test

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xenv"
)

// =============================================================================
// 测试辅助函数
// =============================================================================

// setEnv 设置环境变量用于测试
func setEnv(t testing.TB, value string) {
	t.Helper()
	if err := os.Setenv(xenv.EnvDeployType, value); err != nil {
		t.Fatalf("os.Setenv(%q, %q) failed: %v", xenv.EnvDeployType, value, err)
	}
}

// unsetEnv 删除环境变量用于测试
func unsetEnv(t testing.TB) {
	t.Helper()
	if err := os.Unsetenv(xenv.EnvDeployType); err != nil {
		t.Fatalf("os.Unsetenv(%q) failed: %v", xenv.EnvDeployType, err)
	}
}

// withEnvScope 临时设置环境变量并在测试结束后恢复
func withEnvScope(t testing.TB, value string, shouldSet bool) {
	t.Helper()
	oldValue, hadOldValue := os.LookupEnv(xenv.EnvDeployType)
	t.Cleanup(func() {
		xenv.Reset() // 重置全局状态
		if hadOldValue {
			setEnv(t, oldValue)
		} else {
			unsetEnv(t)
		}
	})
	if shouldSet {
		setEnv(t, value)
	} else {
		unsetEnv(t)
	}
}

// =============================================================================
// DeployType 类型方法测试
// =============================================================================

func TestDeployType_String(t *testing.T) {
	tests := []struct {
		dt   xenv.DeployType
		want string
	}{
		{xenv.DeployLocal, "LOCAL"},
		{xenv.DeploySaaS, "SAAS"},
		{xenv.DeployType("INVALID"), "INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dt.String(); got != tt.want {
				t.Errorf("DeployType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeployType_IsLocal(t *testing.T) {
	tests := []struct {
		dt   xenv.DeployType
		want bool
	}{
		{xenv.DeployLocal, true},
		{xenv.DeploySaaS, false},
		{xenv.DeployType("INVALID"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsLocal(); got != tt.want {
				t.Errorf("DeployType.IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeployType_IsSaaS(t *testing.T) {
	tests := []struct {
		dt   xenv.DeployType
		want bool
	}{
		{xenv.DeployLocal, false},
		{xenv.DeploySaaS, true},
		{xenv.DeployType("INVALID"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsSaaS(); got != tt.want {
				t.Errorf("DeployType.IsSaaS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeployType_IsValid(t *testing.T) {
	tests := []struct {
		dt   xenv.DeployType
		want bool
	}{
		{xenv.DeployLocal, true},
		{xenv.DeploySaaS, true},
		{xenv.DeployType("INVALID"), false},
		{xenv.DeployType("local"), false}, // 区分大小写
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsValid(); got != tt.want {
				t.Errorf("DeployType(%q).IsValid() = %v, want %v", tt.dt, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Parse 测试
// =============================================================================

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  xenv.DeployType
		err   error
	}{
		// LOCAL 变体
		{"LOCAL", xenv.DeployLocal, nil},
		{"local", xenv.DeployLocal, nil},
		{"Local", xenv.DeployLocal, nil},
		{"  LOCAL  ", xenv.DeployLocal, nil},

		// SAAS 变体
		{"SAAS", xenv.DeploySaaS, nil},
		{"saas", xenv.DeploySaaS, nil},
		{"SaaS", xenv.DeploySaaS, nil},
		{"  SAAS  ", xenv.DeploySaaS, nil},

		// 无效值
		{"", "", xenv.ErrInvalidType},
		{"   ", "", xenv.ErrInvalidType},
		{"invalid", "", xenv.ErrInvalidType},
		{"LOCALx", "", xenv.ErrInvalidType},
		{"SAAS2", "", xenv.ErrInvalidType},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := xenv.Parse(tt.input)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("Parse(%q) error = %v, want %v", tt.input, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Init/InitWith/Reset 测试
// =============================================================================

func TestInit(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		unset    bool
		want     xenv.DeployType
		err      error
	}{
		{"LOCAL", "LOCAL", false, xenv.DeployLocal, nil},
		{"local", "local", false, xenv.DeployLocal, nil},
		{"SAAS", "SAAS", false, xenv.DeploySaaS, nil},
		{"saas", "saas", false, xenv.DeploySaaS, nil},
		{"missing", "", true, "", xenv.ErrMissingEnv},
		{"empty", "", false, "", xenv.ErrEmptyEnv},
		{"whitespace_only", "   ", false, "", xenv.ErrEmptyEnv},
		{"invalid", "invalid", false, "", xenv.ErrInvalidType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnvScope(t, tt.envValue, !tt.unset)

			err := xenv.Init()
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("Init() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			if got := xenv.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
			if !xenv.IsInitialized() {
				t.Error("IsInitialized() = false, want true")
			}
		})
	}
}

func TestMustInit(t *testing.T) {
	t.Run("成功初始化不panic", func(t *testing.T) {
		withEnvScope(t, "LOCAL", true)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustInit() panicked: %v", r)
			}
		}()
		xenv.MustInit()
		if xenv.Type() != xenv.DeployLocal {
			t.Errorf("Type() = %q, want %q", xenv.Type(), xenv.DeployLocal)
		}
	})

	t.Run("环境变量缺失时panic", func(t *testing.T) {
		withEnvScope(t, "", false)

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustInit() did not panic")
			}
		}()
		xenv.MustInit()
	})
}

func TestInitWith(t *testing.T) {
	tests := []struct {
		name string
		dt   xenv.DeployType
		err  error
	}{
		{"LOCAL", xenv.DeployLocal, nil},
		{"SAAS", xenv.DeploySaaS, nil},
		{"INVALID", xenv.DeployType("INVALID"), xenv.ErrInvalidType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xenv.Reset()
			t.Cleanup(xenv.Reset)

			err := xenv.InitWith(tt.dt)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("InitWith(%q) error = %v, want %v", tt.dt, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("InitWith(%q) error = %v", tt.dt, err)
			}
			if got := xenv.Type(); got != tt.dt {
				t.Errorf("Type() = %q, want %q", got, tt.dt)
			}
		})
	}
}

func TestReset(t *testing.T) {
	xenv.Reset()
	t.Cleanup(xenv.Reset)

	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		t.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	if !xenv.IsInitialized() {
		t.Fatal("IsInitialized() = false after InitWith")
	}

	xenv.Reset()
	if xenv.IsInitialized() {
		t.Error("IsInitialized() = true after Reset, want false")
	}
	if xenv.Type() != "" {
		t.Errorf("Type() = %q after Reset, want empty", xenv.Type())
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	t.Run("Init重复调用", func(t *testing.T) {
		withEnvScope(t, "LOCAL", true)

		if err := xenv.Init(); err != nil {
			t.Fatalf("first Init() error = %v", err)
		}
		err := xenv.Init()
		if !errors.Is(err, xenv.ErrAlreadyInitialized) {
			t.Errorf("second Init() error = %v, want %v", err, xenv.ErrAlreadyInitialized)
		}
	})

	t.Run("InitWith重复调用", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if err := xenv.InitWith(xenv.DeployLocal); err != nil {
			t.Fatalf("first InitWith() error = %v", err)
		}
		err := xenv.InitWith(xenv.DeploySaaS)
		if !errors.Is(err, xenv.ErrAlreadyInitialized) {
			t.Errorf("second InitWith() error = %v, want %v", err, xenv.ErrAlreadyInitialized)
		}
	})
}

func TestConcurrentInit(t *testing.T) {
	xenv.Reset()
	t.Cleanup(xenv.Reset)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make(chan error, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			errs <- xenv.InitWith(xenv.DeployLocal)
		}()
	}

	wg.Wait()
	close(errs)

	var successes, alreadyInits int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, xenv.ErrAlreadyInitialized):
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

// =============================================================================
// 全局访问函数测试
// =============================================================================

func TestType(t *testing.T) {
	t.Run("未初始化返回空字符串", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if got := xenv.Type(); got != "" {
			t.Errorf("Type() = %q, want empty", got)
		}
	})

	t.Run("已初始化返回正确值", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if err := xenv.InitWith(xenv.DeploySaaS); err != nil {
			t.Fatalf("InitWith(DeploySaaS) error = %v", err)
		}
		if got := xenv.Type(); got != xenv.DeploySaaS {
			t.Errorf("Type() = %q, want %q", got, xenv.DeploySaaS)
		}
	})
}

func TestIsLocal_Global(t *testing.T) {
	tests := []struct {
		name string
		dt   xenv.DeployType
		want bool
	}{
		{"LOCAL", xenv.DeployLocal, true},
		{"SAAS", xenv.DeploySaaS, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xenv.Reset()
			t.Cleanup(xenv.Reset)

			if err := xenv.InitWith(tt.dt); err != nil {
				t.Fatalf("InitWith(%q) error = %v", tt.dt, err)
			}
			if got := xenv.IsLocal(); got != tt.want {
				t.Errorf("IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("未初始化返回false", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if xenv.IsLocal() {
			t.Error("IsLocal() = true, want false (not initialized)")
		}
	})
}

func TestIsSaaS_Global(t *testing.T) {
	tests := []struct {
		name string
		dt   xenv.DeployType
		want bool
	}{
		{"LOCAL", xenv.DeployLocal, false},
		{"SAAS", xenv.DeploySaaS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xenv.Reset()
			t.Cleanup(xenv.Reset)

			if err := xenv.InitWith(tt.dt); err != nil {
				t.Fatalf("InitWith(%q) error = %v", tt.dt, err)
			}
			if got := xenv.IsSaaS(); got != tt.want {
				t.Errorf("IsSaaS() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("未初始化返回false", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if xenv.IsSaaS() {
			t.Error("IsSaaS() = true, want false (not initialized)")
		}
	})
}

func TestIsInitialized(t *testing.T) {
	xenv.Reset()
	t.Cleanup(xenv.Reset)

	if xenv.IsInitialized() {
		t.Error("IsInitialized() = true, want false")
	}

	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		t.Fatalf("InitWith(DeployLocal) error = %v", err)
	}
	if !xenv.IsInitialized() {
		t.Error("IsInitialized() = false, want true")
	}
}

func TestRequireType(t *testing.T) {
	t.Run("未初始化返回错误", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		_, err := xenv.RequireType()
		if !errors.Is(err, xenv.ErrNotInitialized) {
			t.Errorf("RequireType() error = %v, want %v", err, xenv.ErrNotInitialized)
		}
	})

	t.Run("已初始化返回正确值", func(t *testing.T) {
		xenv.Reset()
		t.Cleanup(xenv.Reset)

		if err := xenv.InitWith(xenv.DeploySaaS); err != nil {
			t.Fatalf("InitWith(DeploySaaS) error = %v", err)
		}
		got, err := xenv.RequireType()
		if err != nil {
			t.Fatalf("RequireType() error = %v", err)
		}
		if got != xenv.DeploySaaS {
			t.Errorf("RequireType() = %q, want %q", got, xenv.DeploySaaS)
		}
	})
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	xenv.Reset()
	t.Cleanup(xenv.Reset)

	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		t.Fatalf("InitWith(DeployLocal) error = %v", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			_ = xenv.Type()
			_ = xenv.IsLocal()
			_ = xenv.IsSaaS()
			_ = xenv.IsInitialized()
			_, _ = xenv.RequireType()
		}()
	}

	wg.Wait()
}

// =============================================================================
// 常量测试
// =============================================================================

func TestConstants(t *testing.T) {
	if xenv.EnvDeployType != "DEPLOYMENT_TYPE" {
		t.Errorf("EnvDeployType = %q, want %q", xenv.EnvDeployType, "DEPLOYMENT_TYPE")
	}
	if string(xenv.DeployLocal) != "LOCAL" {
		t.Errorf("DeployLocal = %q, want %q", xenv.DeployLocal, "LOCAL")
	}
	if string(xenv.DeploySaaS) != "SAAS" {
		t.Errorf("DeploySaaS = %q, want %q", xenv.DeploySaaS, "SAAS")
	}
}

// 设计决策: 示例测试集中在 example_test.go 中维护，避免分散在多个文件中
// 增加 API 变更时的更新点。deploy_test.go 仅包含单元测试和 benchmark。
