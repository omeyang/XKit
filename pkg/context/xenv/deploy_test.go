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
	if err := os.Setenv(xenv.EnvDeploymentType, value); err != nil {
		t.Fatalf("os.Setenv(%q, %q) failed: %v", xenv.EnvDeploymentType, value, err)
	}
}

// unsetEnv 删除环境变量用于测试
func unsetEnv(t testing.TB) {
	t.Helper()
	if err := os.Unsetenv(xenv.EnvDeploymentType); err != nil {
		t.Fatalf("os.Unsetenv(%q) failed: %v", xenv.EnvDeploymentType, err)
	}
}

// withEnvScope 临时设置环境变量并在测试结束后恢复
func withEnvScope(t testing.TB, value string, shouldSet bool) {
	t.Helper()
	oldValue, hadOldValue := os.LookupEnv(xenv.EnvDeploymentType)
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
// DeploymentType 类型方法测试
// =============================================================================

func TestDeploymentType_String(t *testing.T) {
	tests := []struct {
		dt   xenv.DeploymentType
		want string
	}{
		{xenv.DeploymentLocal, "LOCAL"},
		{xenv.DeploymentSaaS, "SAAS"},
		{xenv.DeploymentType("INVALID"), "INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dt.String(); got != tt.want {
				t.Errorf("DeploymentType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeploymentType_IsLocal(t *testing.T) {
	tests := []struct {
		dt   xenv.DeploymentType
		want bool
	}{
		{xenv.DeploymentLocal, true},
		{xenv.DeploymentSaaS, false},
		{xenv.DeploymentType("INVALID"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsLocal(); got != tt.want {
				t.Errorf("DeploymentType.IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeploymentType_IsSaaS(t *testing.T) {
	tests := []struct {
		dt   xenv.DeploymentType
		want bool
	}{
		{xenv.DeploymentLocal, false},
		{xenv.DeploymentSaaS, true},
		{xenv.DeploymentType("INVALID"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsSaaS(); got != tt.want {
				t.Errorf("DeploymentType.IsSaaS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeploymentType_IsValid(t *testing.T) {
	tests := []struct {
		dt   xenv.DeploymentType
		want bool
	}{
		{xenv.DeploymentLocal, true},
		{xenv.DeploymentSaaS, true},
		{xenv.DeploymentType("INVALID"), false},
		{xenv.DeploymentType("local"), false}, // 区分大小写
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsValid(); got != tt.want {
				t.Errorf("DeploymentType(%q).IsValid() = %v, want %v", tt.dt, got, tt.want)
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
		want  xenv.DeploymentType
		err   error
	}{
		// LOCAL 变体
		{"LOCAL", xenv.DeploymentLocal, nil},
		{"local", xenv.DeploymentLocal, nil},
		{"Local", xenv.DeploymentLocal, nil},
		{"  LOCAL  ", xenv.DeploymentLocal, nil},

		// SAAS 变体
		{"SAAS", xenv.DeploymentSaaS, nil},
		{"saas", xenv.DeploymentSaaS, nil},
		{"SaaS", xenv.DeploymentSaaS, nil},
		{"  SAAS  ", xenv.DeploymentSaaS, nil},

		// 无效值
		{"", "", xenv.ErrInvalidDeploymentType},
		{"   ", "", xenv.ErrInvalidDeploymentType},
		{"invalid", "", xenv.ErrInvalidDeploymentType},
		{"LOCALx", "", xenv.ErrInvalidDeploymentType},
		{"SAAS2", "", xenv.ErrInvalidDeploymentType},
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
		want     xenv.DeploymentType
		err      error
	}{
		{"LOCAL", "LOCAL", false, xenv.DeploymentLocal, nil},
		{"local", "local", false, xenv.DeploymentLocal, nil},
		{"SAAS", "SAAS", false, xenv.DeploymentSaaS, nil},
		{"saas", "saas", false, xenv.DeploymentSaaS, nil},
		{"missing", "", true, "", xenv.ErrMissingEnv},
		{"empty", "", false, "", xenv.ErrEmptyEnv},
		{"whitespace_only", "   ", false, "", xenv.ErrEmptyEnv},
		{"invalid", "invalid", false, "", xenv.ErrInvalidDeploymentType},
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
		if xenv.Type() != xenv.DeploymentLocal {
			t.Errorf("Type() = %q, want %q", xenv.Type(), xenv.DeploymentLocal)
		}
	})

	t.Run("环境变量缺失时panic", func(t *testing.T) {
		withEnvScope(t, "", false)

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("MustInit() did not panic")
			}
			err, ok := r.(error)
			if !ok {
				t.Fatalf("MustInit() panic value is not error: %v (%T)", r, r)
			}
			if !errors.Is(err, xenv.ErrMissingEnv) {
				t.Errorf("MustInit() panic error = %v, want ErrMissingEnv", err)
			}
		}()
		xenv.MustInit()
	})
}

func TestInitWith(t *testing.T) {
	tests := []struct {
		name string
		dt   xenv.DeploymentType
		err  error
	}{
		{"LOCAL", xenv.DeploymentLocal, nil},
		{"SAAS", xenv.DeploymentSaaS, nil},
		{"INVALID", xenv.DeploymentType("INVALID"), xenv.ErrInvalidDeploymentType},
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

	if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
		t.Fatalf("InitWith(DeploymentLocal) error = %v", err)
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

		if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
			t.Fatalf("first InitWith() error = %v", err)
		}
		err := xenv.InitWith(xenv.DeploymentSaaS)
		if !errors.Is(err, xenv.ErrAlreadyInitialized) {
			t.Errorf("second InitWith() error = %v, want %v", err, xenv.ErrAlreadyInitialized)
		}
	})
}

// TestInitAlreadyInitialized_ErrorPriority 验证错误优先级：
// 已初始化 + 非法/缺失输入 → 始终返回 ErrAlreadyInitialized（而非输入校验错误）。
func TestInitAlreadyInitialized_ErrorPriority(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(t *testing.T) // 初始化后的环境设置
		initFn  func() error       // 第二次初始化调用
	}{
		{
			name: "Init_已初始化+环境缺失",
			setupFn: func(t *testing.T) {
				t.Helper()
				unsetEnv(t)
			},
			initFn: xenv.Init,
		},
		{
			name: "Init_已初始化+环境为空",
			setupFn: func(t *testing.T) {
				t.Helper()
				setEnv(t, "")
			},
			initFn: xenv.Init,
		},
		{
			name: "Init_已初始化+环境非法",
			setupFn: func(t *testing.T) {
				t.Helper()
				setEnv(t, "INVALID")
			},
			initFn: xenv.Init,
		},
		{
			name:    "InitWith_已初始化+参数非法",
			setupFn: func(_ *testing.T) {},
			initFn: func() error {
				return xenv.InitWith(xenv.DeploymentType("INVALID"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xenv.Reset()
			t.Cleanup(xenv.Reset)

			// 先完成首次初始化
			if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
				t.Fatalf("first InitWith() error = %v", err)
			}

			// 修改环境使第二次调用的输入校验会失败
			tt.setupFn(t)

			// 验证：无论输入是否合法，已初始化后始终返回 ErrAlreadyInitialized
			err := tt.initFn()
			if !errors.Is(err, xenv.ErrAlreadyInitialized) {
				t.Errorf("error = %v, want %v (ErrAlreadyInitialized should take priority over input validation errors)", err, xenv.ErrAlreadyInitialized)
			}
		})
	}
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
			errs <- xenv.InitWith(xenv.DeploymentLocal)
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

		if err := xenv.InitWith(xenv.DeploymentSaaS); err != nil {
			t.Fatalf("InitWith(DeploymentSaaS) error = %v", err)
		}
		if got := xenv.Type(); got != xenv.DeploymentSaaS {
			t.Errorf("Type() = %q, want %q", got, xenv.DeploymentSaaS)
		}
	})
}

func TestIsLocal_Global(t *testing.T) {
	tests := []struct {
		name string
		dt   xenv.DeploymentType
		want bool
	}{
		{"LOCAL", xenv.DeploymentLocal, true},
		{"SAAS", xenv.DeploymentSaaS, false},
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
		dt   xenv.DeploymentType
		want bool
	}{
		{"LOCAL", xenv.DeploymentLocal, false},
		{"SAAS", xenv.DeploymentSaaS, true},
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

	if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
		t.Fatalf("InitWith(DeploymentLocal) error = %v", err)
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

		if err := xenv.InitWith(xenv.DeploymentSaaS); err != nil {
			t.Fatalf("InitWith(DeploymentSaaS) error = %v", err)
		}
		got, err := xenv.RequireType()
		if err != nil {
			t.Fatalf("RequireType() error = %v", err)
		}
		if got != xenv.DeploymentSaaS {
			t.Errorf("RequireType() = %q, want %q", got, xenv.DeploymentSaaS)
		}
	})
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	xenv.Reset()
	t.Cleanup(xenv.Reset)

	if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
		t.Fatalf("InitWith(DeploymentLocal) error = %v", err)
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

// TestConcurrentResetAndRead 验证 Reset 与 RequireType/Type 并发时的语义契约：
// RequireType 绝不返回 ("", nil)，Type 在 Reset 期间返回空字符串是合法行为。
//
// 回归守护目标：deploy.go:235 的 dt == "" 后置校验。若误删该校验，
// RequireType 可能在 TOCTOU 窗口中返回 ("", nil)，本测试将捕获此回退。
func TestConcurrentResetAndRead(t *testing.T) {
	const iterations = 1000
	const readers = 10

	for range iterations {
		xenv.Reset()
		if err := xenv.InitWith(xenv.DeploymentLocal); err != nil {
			t.Fatalf("InitWith(DeploymentLocal) error = %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(readers + 1)

		// 写者：Reset
		go func() {
			defer wg.Done()
			xenv.Reset()
		}()

		// 读者：RequireType + Type
		for range readers {
			go func() {
				defer wg.Done()

				dt, err := xenv.RequireType()
				if err == nil && dt == "" {
					t.Errorf("RequireType() = (%q, nil), contract violation: must not return empty type without error", dt)
				}

				if got := xenv.Type(); got != "" && got != xenv.DeploymentLocal {
					t.Errorf("Type() = %q, want empty or DeploymentLocal", got)
				}
			}()
		}

		wg.Wait()
	}
	xenv.Reset()
}

// =============================================================================
// 常量测试
// =============================================================================

func TestConstants(t *testing.T) {
	if xenv.EnvDeploymentType != "DEPLOYMENT_TYPE" {
		t.Errorf("EnvDeploymentType = %q, want %q", xenv.EnvDeploymentType, "DEPLOYMENT_TYPE")
	}
	if string(xenv.DeploymentLocal) != "LOCAL" {
		t.Errorf("DeploymentLocal = %q, want %q", xenv.DeploymentLocal, "LOCAL")
	}
	if string(xenv.DeploymentSaaS) != "SAAS" {
		t.Errorf("DeploymentSaaS = %q, want %q", xenv.DeploymentSaaS, "SAAS")
	}
}

// =============================================================================
// Fuzz 测试
// =============================================================================

// FuzzParse 模糊测试验证 Parse 对任意输入不 panic、错误语义一致。
//
// 与 xctx.FuzzParseDeploymentType 互补：xctx 测试 deploy.Parse 本身，
// 本测试验证 xenv.Parse 的错误包装逻辑（ErrInvalidDeploymentType 哨兵）。
func FuzzParse(f *testing.F) {
	f.Add("LOCAL")
	f.Add("SAAS")
	f.Add("local")
	f.Add("saas")
	f.Add("")
	f.Add("   ")
	f.Add("invalid")

	f.Fuzz(func(t *testing.T, input string) {
		dt, err := xenv.Parse(input)
		if err != nil {
			if !errors.Is(err, xenv.ErrInvalidDeploymentType) {
				t.Errorf("Parse(%q) error = %v, want ErrInvalidDeploymentType", input, err)
			}
			return
		}
		if !dt.IsValid() {
			t.Errorf("Parse(%q) returned invalid type %q without error", input, dt)
		}
	})
}

// 设计决策: 示例测试集中在 example_test.go 中维护，避免分散在多个文件中
// 增加 API 变更时的更新点。deploy_test.go 仅包含单元测试和 benchmark。
