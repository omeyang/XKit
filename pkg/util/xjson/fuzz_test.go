package xjson

import (
	"encoding/json"
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
		// Pretty should never panic
		result := Pretty(s)
		if result == "" {
			t.Error("Pretty returned empty string")
		}
		// 对 string 类型，Pretty 应该返回合法的 JSON
		if !json.Valid([]byte(result)) {
			t.Errorf("Pretty(%q) produced invalid JSON: %s", s, result)
		}
	})
}

func FuzzPrettyBytes(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{0xFF, 0xFE})
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, b []byte) {
		// Pretty should never panic for []byte input
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
