package xctx

import (
	"context"
	"errors"
	"testing"
)

// =============================================================================
// GetDeploymentType 私有分支测试
// =============================================================================

// TestGetDeploymentType_TypeSwitch 测试 GetDeploymentType 中的 type switch 分支
// 这些分支需要直接使用 context.WithValue 绕过 WithDeploymentType 的验证
func TestGetDeploymentType_TypeSwitch(t *testing.T) {
	t.Run("invalid DeploymentType stored directly", func(t *testing.T) {
		// 直接存储无效的 DeploymentType
		ctx := context.WithValue(context.Background(), keyDeploymentType, DeploymentType("INVALID"))
		_, err := GetDeploymentType(ctx)
		if !errors.Is(err, ErrInvalidDeploymentType) {
			t.Errorf("GetDeploymentType() error = %v, want %v", err, ErrInvalidDeploymentType)
		}
	})

	t.Run("valid string stored directly", func(t *testing.T) {
		// 直接存储有效的字符串（会被解析为 DeploymentType）
		ctx := context.WithValue(context.Background(), keyDeploymentType, "LOCAL")
		dt, err := GetDeploymentType(ctx)
		if err != nil {
			t.Fatalf("GetDeploymentType() error = %v", err)
		}
		if dt != DeploymentLocal {
			t.Errorf("GetDeploymentType() = %q, want %q", dt, DeploymentLocal)
		}
	})

	t.Run("invalid string stored directly", func(t *testing.T) {
		// 直接存储无效的字符串
		ctx := context.WithValue(context.Background(), keyDeploymentType, "invalid")
		_, err := GetDeploymentType(ctx)
		if !errors.Is(err, ErrInvalidDeploymentType) {
			t.Errorf("GetDeploymentType() error = %v, want %v", err, ErrInvalidDeploymentType)
		}
	})

	t.Run("other type stored directly", func(t *testing.T) {
		// 直接存储其他类型（int）
		ctx := context.WithValue(context.Background(), keyDeploymentType, 123)
		_, err := GetDeploymentType(ctx)
		if !errors.Is(err, ErrInvalidDeploymentType) {
			t.Errorf("GetDeploymentType() error = %v, want %v", err, ErrInvalidDeploymentType)
		}
	})
}
