package xconf

import (
	"strings"
	"testing"
)

func FuzzNewFromBytes(f *testing.F) {
	f.Add([]byte("key: value\n"), "yaml")
	f.Add([]byte(`{"key":"value"}`), "json")

	f.Fuzz(func(t *testing.T, data []byte, format string) {
		if len(data) == 0 {
			return
		}

		switch strings.ToLower(format) {
		case "yaml", "yml":
			format = string(FormatYAML)
		case "json":
			format = string(FormatJSON)
		default:
			return
		}

		cfg, err := NewFromBytes(data, Format(format))
		if err != nil {
			return
		}

		var out map[string]any
		if err := cfg.Unmarshal("", &out); err != nil {
			return
		}
	})
}
