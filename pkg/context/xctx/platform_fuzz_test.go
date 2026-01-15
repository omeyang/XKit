package xctx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// FuzzWithUnclassRegionID 模糊测试 WithUnclassRegionID 函数
//
// 测试目标：
//   - 确保任意字符串输入不会导致 panic
//   - 验证 context 注入和读取的一致性
//   - 验证特殊字符（空字符、换行、Unicode 等）的处理
func FuzzWithUnclassRegionID(f *testing.F) {
	// 种子数据：覆盖常见值、边界情况和特殊字符
	seeds := []string{
		"", " ", "  ",
		"region-001", "REGION-001",
		"region_with_underscore",
		"region.with.dots",
		"region/with/slash",
		"region\twith\ttabs",
		"region\nwith\nnewlines",
		"region\x00with\x00nulls",
		"中文区域",
		"🌍emoji",
		string(make([]byte, 1024)), // 长字符串
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		ctx := context.Background()

		// 注入值
		newCtx, err := xctx.WithUnclassRegionID(ctx, input)
		if err != nil {
			t.Fatalf("WithUnclassRegionID(%q) unexpected error: %v", truncate(input), err)
		}

		// 读取值
		got := xctx.UnclassRegionID(newCtx)

		// 核心不变式：写入和读取必须一致
		if got != input {
			t.Errorf("UnclassRegionID mismatch: got %q, want %q", truncate(got), truncate(input))
		}
	})
}

// FuzzPlatformRoundTrip 模糊测试 Platform 结构体的完整写入/读取周期
//
// 测试目标：
//   - 确保 WithPlatform 和 GetPlatform 的一致性
//   - 验证 HasParent 和 UnclassRegionID 的组合场景
func FuzzPlatformRoundTrip(f *testing.F) {
	// 种子数据
	seeds := []struct {
		hasParent bool
		regionID  string
	}{
		{true, "region-001"},
		{false, "region-002"},
		{true, ""},
		{false, ""},
		{true, "  "},
		{false, "\x00"},
		{true, "very-long-" + string(make([]byte, 256))},
	}
	for _, s := range seeds {
		f.Add(s.hasParent, s.regionID)
	}

	f.Fuzz(func(t *testing.T, hasParent bool, regionID string) {
		ctx := context.Background()
		p := xctx.Platform{
			HasParent:       hasParent,
			UnclassRegionID: regionID,
		}

		// 注入 Platform
		newCtx, err := xctx.WithPlatform(ctx, p)
		if err != nil {
			t.Fatalf("WithPlatform unexpected error: %v", err)
		}

		// 读取 Platform
		got := xctx.GetPlatform(newCtx)

		// 核心不变式：HasParent 必须一致
		if got.HasParent != hasParent {
			t.Errorf("HasParent mismatch: got %v, want %v", got.HasParent, hasParent)
		}

		// 核心不变式：非空 regionID 必须一致
		// 注意：WithPlatform 仅在 regionID 非空时注入
		if regionID != "" {
			if got.UnclassRegionID != regionID {
				t.Errorf("UnclassRegionID mismatch: got %q, want %q",
					truncate(got.UnclassRegionID), truncate(regionID))
			}
		} else {
			// regionID 为空时，GetPlatform 应返回空字符串
			if got.UnclassRegionID != "" {
				t.Errorf("UnclassRegionID should be empty, got %q", truncate(got.UnclassRegionID))
			}
		}

		// 验证 HasParent 的 ok 标志
		_, ok := xctx.HasParent(newCtx)
		if !ok {
			t.Error("HasParent should be set after WithPlatform")
		}
	})
}

// FuzzRequireHasParent 模糊测试 RequireHasParent 错误处理
//
// 测试目标：
//   - 验证 HasParent 存在时返回正确值
//   - 验证 HasParent 不存在时返回正确错误
func FuzzRequireHasParent(f *testing.F) {
	// 种子数据：测试两种状态
	f.Add(true, true)   // 设置为 true
	f.Add(true, false)  // 设置为 false
	f.Add(false, true)  // 不设置
	f.Add(false, false) // 不设置

	f.Fuzz(func(t *testing.T, shouldSet bool, value bool) {
		ctx := context.Background()

		if shouldSet {
			ctx, _ = xctx.WithHasParent(ctx, value)
		}

		got, err := xctx.RequireHasParent(ctx)

		if shouldSet {
			// 已设置：应成功
			if err != nil {
				t.Errorf("RequireHasParent should succeed when set, got error: %v", err)
			}
			if got != value {
				t.Errorf("RequireHasParent got %v, want %v", got, value)
			}
		} else {
			// 未设置：应返回 ErrMissingHasParent
			if err == nil {
				t.Error("RequireHasParent should fail when not set")
			}
			if !errors.Is(err, xctx.ErrMissingHasParent) {
				t.Errorf("RequireHasParent error = %v, want ErrMissingHasParent", err)
			}
		}
	})
}

// truncate 截断长字符串用于错误信息显示
func truncate(s string) string {
	const maxLen = 32
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
