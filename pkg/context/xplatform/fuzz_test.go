package xplatform

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

// FuzzConfigValidate 对 Config.Validate 进行模糊测试。
//
// 目标：
//   - 不能 panic
//   - 返回错误必须属于已声明的错误集合
//   - 校验语义不变量：纯空白 PlatformID 等价于空；超过最大长度必报错
func FuzzConfigValidate(f *testing.F) {
	seeds := []struct {
		platform string
		region   string
	}{
		{"", ""},
		{"   ", ""},
		{"platform-1", ""},
		{"platform-1", "region-1"},
		{"plat form", ""},     // 含空格
		{"plat\tform", ""},    // 含制表符
		{"plat\x00form", ""},  // 含 NUL
		{"plat\x07form", ""},  // 含 BEL
		{"plat\nform", ""},    // 含换行
		{"\u00a0platform", ""}, // 含 NBSP（unicode space）
		{strings.Repeat("a", 128), ""},
		{strings.Repeat("a", 129), ""},
		{"platform", strings.Repeat("b", 128)},
		{"platform", strings.Repeat("b", 129)},
		{"platform", "  "},
		{"平台", "区域"},
	}
	for _, s := range seeds {
		f.Add(s.platform, s.region)
	}

	allowed := []error{ErrMissingPlatformID, ErrInvalidPlatformID, ErrInvalidUnclassRegionID}
	isAllowed := func(err error) bool {
		for _, e := range allowed {
			if errors.Is(err, e) {
				return true
			}
		}
		return false
	}

	f.Fuzz(func(t *testing.T, platformID, regionID string) {
		cfg := Config{PlatformID: platformID, UnclassRegionID: regionID}
		err := cfg.Validate()

		if err != nil && !isAllowed(err) {
			t.Fatalf("unexpected error type for platform=%q region=%q: %v", platformID, regionID, err)
		}

		// 不变量 1：纯空白 PlatformID 必须报 ErrMissingPlatformID
		if strings.TrimSpace(platformID) == "" {
			if !errors.Is(err, ErrMissingPlatformID) {
				t.Fatalf("blank PlatformID %q should return ErrMissingPlatformID, got %v", platformID, err)
			}
			return
		}

		// 不变量 2：PlatformID 包含空白/控制/非ASCII字符必须报 ErrInvalidPlatformID
		if strings.ContainsFunc(platformID, unicode.IsSpace) || strings.ContainsFunc(platformID, unicode.IsControl) || hasNonPrintableASCII(platformID) {
			if !errors.Is(err, ErrInvalidPlatformID) {
				t.Fatalf("PlatformID %q with invalid bytes should return ErrInvalidPlatformID, got %v", platformID, err)
			}
			return
		}

		// 不变量 3：超长 PlatformID 必报 ErrInvalidPlatformID
		if len(platformID) > maxPlatformIDLen {
			if !errors.Is(err, ErrInvalidPlatformID) {
				t.Fatalf("PlatformID len=%d should return ErrInvalidPlatformID, got %v", len(platformID), err)
			}
			return
		}

		// 此时 PlatformID 合法，错误（如有）必须来自 RegionID
		if err != nil && !errors.Is(err, ErrInvalidUnclassRegionID) {
			t.Fatalf("with valid PlatformID, only RegionID errors expected, got %v", err)
		}

		// 不变量 4：纯空白 RegionID 视为未设置，应通过校验
		if strings.TrimSpace(regionID) == "" {
			if err != nil {
				t.Fatalf("blank regionID %q should be treated as unset, got %v", regionID, err)
			}
		}
	})
}

// hasNonPrintableASCII 判断字符串是否包含非可打印 ASCII 字节。
//
// 与 platform.go 的 containsNonPrintableASCII 语义一致，仅用于 fuzz 断言。
func hasNonPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x21 || b > 0x7e {
			return true
		}
	}
	return false
}
