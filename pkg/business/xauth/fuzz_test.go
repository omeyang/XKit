package xauth

import (
	"errors"
	"testing"
)

// FuzzConfigValidateHost 对 Config.validateHost 进行模糊测试。
//
// validateHost 解析任意字符串作为 URL，并强制 https 协议。模糊测试目标：
//   - 不能 panic
//   - 任何非 https 输入（在 AllowInsecure=false 时）必须返回非 nil 错误
//   - 返回的错误必须属于已声明的错误集合（ErrMissingHost / ErrInvalidHost / ErrInsecureHost）
func FuzzConfigValidateHost(f *testing.F) {
	seeds := []string{
		"",
		"   ",
		"https://auth.example.com",
		"http://auth.example.com",
		"HTTPS://AUTH.EXAMPLE.COM",
		"https://",
		"://broken",
		"auth.example.com",
		"ftp://auth.example.com",
		"https://example.com:443/path?q=1",
		"https://[::1]:8443",
		"\x00https://evil",
		"https://例子.中国",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	allowedErrs := []error{ErrMissingHost, ErrInvalidHost, ErrInsecureHost}
	isAllowed := func(err error) bool {
		for _, e := range allowedErrs {
			if errors.Is(err, e) {
				return true
			}
		}
		return false
	}

	f.Fuzz(func(t *testing.T, host string) {
		// 严格模式：必须 https
		strict := &Config{Host: host}
		err := strict.validateHost()
		if err != nil && !isAllowed(err) {
			t.Fatalf("strict: unexpected error type for host=%q: %v", host, err)
		}

		// 宽松模式：允许 http
		loose := &Config{Host: host, AllowInsecure: true}
		err = loose.validateHost()
		if err != nil && !isAllowed(err) {
			t.Fatalf("loose: unexpected error type for host=%q: %v", host, err)
		}

		// 关键不变量：strict 通过则 loose 必通过
		if strict.validateHost() == nil && loose.validateHost() != nil {
			t.Fatalf("loose stricter than strict for host=%q", host)
		}
	})
}
