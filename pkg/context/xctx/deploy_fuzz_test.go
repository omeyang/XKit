package xctx_test

import (
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// FuzzParseDeploymentType 模糊测试 ParseDeploymentType 函数
//
// 测试目标：
//   - 确保任意输入不会导致 panic
//   - 验证有效输入（LOCAL/SAAS 及其变体）返回正确结果
//   - 验证无效输入返回预期错误类型
func FuzzParseDeploymentType(f *testing.F) {
	// 种子数据：覆盖有效值、边界情况和典型无效值
	seeds := []string{
		"LOCAL", "local", "Local", "SAAS", "saas", "SaaS",
		"", "  ", "INVALID", "LOCALx", "SAAS2",
		"  LOCAL  ", "  SAAS  ",
		"\tLOCAL\n", "LOCAL\x00", // 特殊字符
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		dt, err := xctx.ParseDeploymentType(input)

		// 核心不变式：返回值组合必须一致
		if err == nil {
			// 成功时必须返回有效类型
			if !dt.IsValid() {
				t.Errorf("ParseDeploymentType(%q) = %q (invalid), want valid type", input, dt)
			}
			// 成功时只能是 LOCAL 或 SAAS
			if dt != xctx.DeploymentLocal && dt != xctx.DeploymentSaaS {
				t.Errorf("ParseDeploymentType(%q) = %q, want LOCAL or SAAS", input, dt)
			}
		} else {
			// 失败时必须返回空字符串
			if dt != "" {
				t.Errorf("ParseDeploymentType(%q) error = %v, but dt = %q (want empty)", input, err, dt)
			}
			// 错误类型必须是预期的哨兵错误之一
			if !errors.Is(err, xctx.ErrMissingDeploymentTypeValue) &&
				!errors.Is(err, xctx.ErrInvalidDeploymentType) {
				t.Errorf("ParseDeploymentType(%q) error = %v, want ErrMissingDeploymentTypeValue or ErrInvalidDeploymentType", input, err)
			}
		}
	})
}
