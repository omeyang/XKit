package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// DeploymentType 类型方法测试
// =============================================================================

func TestDeploymentType_String(t *testing.T) {
	tests := []struct {
		dt   xctx.DeploymentType
		want string
	}{
		{xctx.DeploymentLocal, "LOCAL"},
		{xctx.DeploymentSaaS, "SAAS"},
		{xctx.DeploymentType("INVALID"), "INVALID"},
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
		dt   xctx.DeploymentType
		want bool
	}{
		{xctx.DeploymentLocal, true},
		{xctx.DeploymentSaaS, false},
		{xctx.DeploymentType("INVALID"), false},
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
		dt   xctx.DeploymentType
		want bool
	}{
		{xctx.DeploymentLocal, false},
		{xctx.DeploymentSaaS, true},
		{xctx.DeploymentType("INVALID"), false},
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
		dt   xctx.DeploymentType
		want bool
	}{
		{xctx.DeploymentLocal, true},
		{xctx.DeploymentSaaS, true},
		{xctx.DeploymentType("INVALID"), false},
		{xctx.DeploymentType("local"), false}, // 区分大小写
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
// Context 操作测试
// =============================================================================

func TestWithDeploymentType(t *testing.T) {
	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.WithDeploymentType(nilCtx, xctx.DeploymentLocal)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithDeploymentType(nil, ...) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})

	tests := []struct {
		name    string
		dt      xctx.DeploymentType
		wantErr bool
	}{
		{"LOCAL", xctx.DeploymentLocal, false},
		{"SAAS", xctx.DeploymentSaaS, false},
		{"INVALID", xctx.DeploymentType("INVALID"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := xctx.WithDeploymentType(context.Background(), tt.dt)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("WithDeploymentType() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("WithDeploymentType() error = %v", err)
			}

			got, err := xctx.GetDeploymentType(ctx)
			if err != nil {
				t.Fatalf("GetDeploymentType() error = %v", err)
			}
			if got != tt.dt {
				t.Errorf("GetDeploymentType() = %q, want %q", got, tt.dt)
			}
		})
	}
}

func TestGetDeploymentType(t *testing.T) {
	t.Run("空context返回错误", func(t *testing.T) {
		_, err := xctx.GetDeploymentType(context.Background())
		if !errors.Is(err, xctx.ErrMissingDeploymentType) {
			t.Errorf("GetDeploymentType(empty) error = %v, want %v", err, xctx.ErrMissingDeploymentType)
		}
	})

	t.Run("nil context返回错误", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.GetDeploymentType(nilCtx)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("GetDeploymentType(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})

	t.Run("覆盖写入返回新值", func(t *testing.T) {
		ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentLocal)
		if err != nil {
			t.Fatalf("WithDeploymentType() error = %v", err)
		}
		ctx, err = xctx.WithDeploymentType(ctx, xctx.DeploymentSaaS)
		if err != nil {
			t.Fatalf("WithDeploymentType(overwrite) error = %v", err)
		}
		got, err := xctx.GetDeploymentType(ctx)
		if err != nil {
			t.Fatalf("GetDeploymentType() error = %v", err)
		}
		if got != xctx.DeploymentSaaS {
			t.Errorf("GetDeploymentType(overwrite) = %q, want %q", got, xctx.DeploymentSaaS)
		}
	})
}

// =============================================================================
// 便捷判断函数测试
// =============================================================================

func TestDeploymentTypeCheckers(t *testing.T) {
	tests := []struct {
		name     string
		trueFor  xctx.DeploymentType // 应返回 true 的类型
		falseFor xctx.DeploymentType // 应返回 false 的类型
		checker  func(context.Context) (bool, error)
	}{
		{
			name:     "IsLocal",
			trueFor:  xctx.DeploymentLocal,
			falseFor: xctx.DeploymentSaaS,
			checker:  xctx.IsLocal,
		},
		{
			name:     "IsSaaS",
			trueFor:  xctx.DeploymentSaaS,
			falseFor: xctx.DeploymentLocal,
			checker:  xctx.IsSaaS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("匹配类型返回true", func(t *testing.T) {
				ctx, err := xctx.WithDeploymentType(context.Background(), tt.trueFor)
				if err != nil {
					t.Fatalf("WithDeploymentType() error = %v", err)
				}
				ok, err := tt.checker(ctx)
				if err != nil {
					t.Fatalf("%s() error = %v", tt.name, err)
				}
				if !ok {
					t.Errorf("%s() should return true for %s", tt.name, tt.trueFor)
				}
			})

			t.Run("不匹配类型返回false", func(t *testing.T) {
				ctx, err := xctx.WithDeploymentType(context.Background(), tt.falseFor)
				if err != nil {
					t.Fatalf("WithDeploymentType() error = %v", err)
				}
				ok, err := tt.checker(ctx)
				if err != nil {
					t.Fatalf("%s() error = %v", tt.name, err)
				}
				if ok {
					t.Errorf("%s() should return false for %s", tt.name, tt.falseFor)
				}
			})

			t.Run("空context返回错误", func(t *testing.T) {
				_, err := tt.checker(context.Background())
				if !errors.Is(err, xctx.ErrMissingDeploymentType) {
					t.Errorf("%s(empty) error = %v, want %v", tt.name, err, xctx.ErrMissingDeploymentType)
				}
			})
		})
	}
}

// =============================================================================
// ParseDeploymentType 测试
// =============================================================================

func TestParseDeploymentType(t *testing.T) {
	tests := []struct {
		input string
		want  xctx.DeploymentType
		err   error
	}{
		// LOCAL 变体
		{"LOCAL", xctx.DeploymentLocal, nil},
		{"local", xctx.DeploymentLocal, nil},
		{"Local", xctx.DeploymentLocal, nil},
		{"  LOCAL  ", xctx.DeploymentLocal, nil},

		// SAAS 变体
		{"SAAS", xctx.DeploymentSaaS, nil},
		{"saas", xctx.DeploymentSaaS, nil},
		{"SaaS", xctx.DeploymentSaaS, nil},
		{"  SAAS  ", xctx.DeploymentSaaS, nil},

		// 无效值
		{"", "", xctx.ErrMissingDeploymentTypeValue},
		{"invalid", "", xctx.ErrInvalidDeploymentType},
		{"LOCALx", "", xctx.ErrInvalidDeploymentType},
		{"SAAS2", "", xctx.ErrInvalidDeploymentType},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := xctx.ParseDeploymentType(tt.input)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("ParseDeploymentType(%q) error = %v, want %v", tt.input, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDeploymentType(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseDeploymentType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Key 常量测试
// =============================================================================

func TestDeploymentKeyConstants(t *testing.T) {
	if xctx.KeyDeploymentType != "deployment_type" {
		t.Errorf("KeyDeploymentType = %q, want %q", xctx.KeyDeploymentType, "deployment_type")
	}
	if xctx.EnvDeploymentType != "DEPLOYMENT_TYPE" {
		t.Errorf("EnvDeploymentType = %q, want %q", xctx.EnvDeploymentType, "DEPLOYMENT_TYPE")
	}
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleWithDeploymentType() {
	ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentSaaS)
	if err != nil {
		return
	}
	dt, err := xctx.GetDeploymentType(ctx)
	if err != nil {
		return
	}
	fmt.Println("DeploymentType:", dt)
	fmt.Println("IsLocal:", dt.IsLocal())
	fmt.Println("IsSaaS:", dt.IsSaaS())
	// Output:
	// DeploymentType: SAAS
	// IsLocal: false
	// IsSaaS: true
}

func ExampleIsLocal() {
	ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentLocal)
	if err != nil {
		return
	}
	ok, err := xctx.IsLocal(ctx)
	if err != nil {
		return
	}
	if ok {
		fmt.Println("Running in local/private deployment")
	}
	// Output:
	// Running in local/private deployment
}

func ExampleIsSaaS() {
	ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentSaaS)
	if err != nil {
		return
	}
	ok, err := xctx.IsSaaS(ctx)
	if err != nil {
		return
	}
	if ok {
		fmt.Println("Running in SaaS cloud deployment")
	}
	// Output:
	// Running in SaaS cloud deployment
}

func ExampleParseDeploymentType() {
	// 支持大小写不敏感
	dt1, err := xctx.ParseDeploymentType("local")
	if err != nil {
		return
	}
	dt2, err := xctx.ParseDeploymentType("SAAS")
	if err != nil {
		return
	}
	_, err = xctx.ParseDeploymentType("invalid")

	fmt.Println("local ->", dt1)
	fmt.Println("SAAS ->", dt2)
	fmt.Println("invalid err ->", err != nil)
	// Output:
	// local -> LOCAL
	// SAAS -> SAAS
	// invalid err -> true
}
