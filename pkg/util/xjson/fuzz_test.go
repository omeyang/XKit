package xjson

import (
	"encoding/json"
	"errors"
	"testing"
)

func FuzzPretty(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add("special chars: <>&\"'")
	f.Add("中文字符串")
	f.Add("\x00\x01\x02") // 控制字符
	f.Add("a\nb\tc")      // 含换行和 tab

	f.Fuzz(func(t *testing.T, s string) {
		// Pretty 不应 panic
		result := Pretty(s)
		if result == "" {
			t.Error("Pretty returned empty string")
		}
		// 对 string 类型，Pretty 应返回合法的 JSON
		if !json.Valid([]byte(result)) {
			t.Errorf("Pretty(%q) produced invalid JSON: %s", s, result)
		}
	})
}

// FuzzPrettyE 验证 PrettyE 的成功路径不变量：对 string 输入，
// json.MarshalIndent 始终成功，结果必须是合法 JSON。
// 错误分支为防御性检查（string 输入不会触发），错误契约
// 由表驱动测试 TestPrettyE 的 error_NaN/error_channel 用例覆盖。
func FuzzPrettyE(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add("special chars: <>&\"'")
	f.Add("中文字符串")
	f.Add("\x00\x01\x02")
	f.Add("a\nb\tc")

	f.Fuzz(func(t *testing.T, s string) {
		got, err := PrettyE(s)
		if err != nil {
			if !errors.Is(err, ErrMarshal) {
				t.Errorf("PrettyE(%q) error should wrap ErrMarshal, got: %v", s, err)
			}
			if got != "" {
				t.Errorf("PrettyE(%q) should return empty string on error, got: %q", s, got)
			}
			return
		}
		if !json.Valid([]byte(got)) {
			t.Errorf("PrettyE(%q) produced invalid JSON: %s", s, got)
		}
	})
}

func FuzzPrettyBytes(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{0xFF, 0xFE})
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, b []byte) {
		// Pretty 对 []byte 输入不应 panic
		result := Pretty(b)
		if result == "" {
			t.Error("Pretty returned empty string for []byte")
		}
		// []byte 通过 encoding/json 序列化为 base64，结果应为合法 JSON
		if !json.Valid([]byte(result)) {
			t.Errorf("Pretty(%v) produced invalid JSON: %s", b, result)
		}
	})
}
