package xpulsar

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authDataGetter 对应 pulsar-client-go 的 auth.Provider.GetData 方法。
// 因 pulsar.Authentication 是 interface{}，此处用本地接口做类型断言，
// 用于验证 token 值已被 xpulsar 在进入 SDK 之前正确 trim。
type authDataGetter interface {
	GetData() ([]byte, error)
}

// =============================================================================
// Token 测试
// =============================================================================

func TestToken(t *testing.T) {
	t.Parallel()

	t.Run("valid token", func(t *testing.T) {
		method, err := Token("my-secret-token")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := Token("")
		require.ErrorIs(t, err, ErrEmptyToken)
	})

	t.Run("whitespace only token", func(t *testing.T) {
		_, err := Token("   ")
		require.ErrorIs(t, err, ErrEmptyToken)
	})

	// 回归测试: 保证 token 前后空白被去除后再传入 SDK，
	// 避免 Token(" foo ") 通过校验却把脏值存进 SDK 的 bug。
	t.Run("surrounding whitespace is trimmed before reaching SDK", func(t *testing.T) {
		method, err := Token("  my-secret  ")
		require.NoError(t, err)

		dg, ok := method.Authentication().(authDataGetter)
		require.True(t, ok, "expected pulsar.Authentication to implement GetData")

		data, err := dg.GetData()
		require.NoError(t, err)
		assert.Equal(t, "my-secret", string(data), "stored token must be trimmed")
	})
}

func TestTokenFromFile(t *testing.T) {
	t.Parallel()

	t.Run("valid path", func(t *testing.T) {
		method, err := TokenFromFile("/etc/pulsar/token")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := TokenFromFile("")
		require.ErrorIs(t, err, ErrEmptyTokenFilePath)
	})

	t.Run("whitespace only path", func(t *testing.T) {
		_, err := TokenFromFile("   ")
		require.ErrorIs(t, err, ErrEmptyTokenFilePath)
	})
}

func TestTokenFromEnv(t *testing.T) {
	// 不使用 t.Parallel()，因为子测试使用 t.Setenv 修改环境变量。
	t.Run("valid env key", func(t *testing.T) {
		method, err := TokenFromEnv("PULSAR_TOKEN")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("env key with value", func(t *testing.T) {
		t.Setenv("XPULSAR_TEST_TOKEN", "my-token")
		method, err := TokenFromEnv("XPULSAR_TEST_TOKEN")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("empty env key", func(t *testing.T) {
		_, err := TokenFromEnv("")
		require.ErrorIs(t, err, ErrEmptyEnvKey)
	})

	t.Run("whitespace only env key", func(t *testing.T) {
		_, err := TokenFromEnv("   ")
		require.ErrorIs(t, err, ErrEmptyEnvKey)
	})

	// 回归测试: envKey 前后空白必须被 trim 后再 os.Getenv，
	// 否则 `TokenFromEnv(" PULSAR_TOKEN ")` 会查找名为 " PULSAR_TOKEN " 的环境变量，
	// 找不到而静默返回空 token，运行时才以认证失败形式暴露。
	t.Run("surrounding whitespace in env key is trimmed before lookup", func(t *testing.T) {
		t.Setenv("XPULSAR_TEST_ENV_TRIM", "env-token-value")

		method, err := TokenFromEnv("  XPULSAR_TEST_ENV_TRIM  ")
		require.NoError(t, err)

		dg, ok := method.Authentication().(authDataGetter)
		require.True(t, ok, "expected pulsar.Authentication to implement GetData")

		data, err := dg.GetData()
		require.NoError(t, err, "os.Getenv must have been called with trimmed key")
		assert.Equal(t, "env-token-value", string(data))
	})
}

func TestTokenFromSupplier(t *testing.T) {
	t.Parallel()

	t.Run("valid supplier", func(t *testing.T) {
		method, err := TokenFromSupplier(func() (string, error) {
			return "dynamic-token", nil
		})
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("nil supplier", func(t *testing.T) {
		_, err := TokenFromSupplier(nil)
		require.ErrorIs(t, err, ErrNilSupplier)
	})
}

// =============================================================================
// TLS 测试
// =============================================================================

func TestTLSCert(t *testing.T) {
	t.Parallel()

	t.Run("valid paths", func(t *testing.T) {
		method, err := TLSCert("/path/to/cert.pem", "/path/to/key.pem")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("empty cert path", func(t *testing.T) {
		_, err := TLSCert("", "/path/to/key.pem")
		require.ErrorIs(t, err, ErrEmptyCertPath)
	})

	t.Run("empty key path", func(t *testing.T) {
		_, err := TLSCert("/path/to/cert.pem", "")
		require.ErrorIs(t, err, ErrEmptyKeyPath)
	})

	t.Run("whitespace only cert path", func(t *testing.T) {
		_, err := TLSCert("   ", "/path/to/key.pem")
		require.ErrorIs(t, err, ErrEmptyCertPath)
	})

	t.Run("both paths empty", func(t *testing.T) {
		// 校验顺序: certPath 先于 keyPath
		_, err := TLSCert("", "")
		require.ErrorIs(t, err, ErrEmptyCertPath)
	})
}

func TestTLSCertFromSupplier(t *testing.T) {
	t.Parallel()

	t.Run("valid supplier", func(t *testing.T) {
		method, err := TLSCertFromSupplier(func() (*tls.Certificate, error) {
			return &tls.Certificate{}, nil
		})
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("nil supplier", func(t *testing.T) {
		_, err := TLSCertFromSupplier(nil)
		require.ErrorIs(t, err, ErrNilSupplier)
	})
}

// =============================================================================
// OAuth2 测试
// =============================================================================

func TestOAuth2(t *testing.T) {
	t.Parallel()

	t.Run("valid params", func(t *testing.T) {
		method, err := OAuth2("https://issuer.example.com", "my-audience", "/path/to/creds.json")
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("empty issuer URL", func(t *testing.T) {
		_, err := OAuth2("", "audience", "/path")
		require.ErrorIs(t, err, ErrEmptyIssuerURL)
	})

	t.Run("empty audience", func(t *testing.T) {
		_, err := OAuth2("https://issuer", "", "/path")
		require.ErrorIs(t, err, ErrEmptyAudience)
	})

	t.Run("empty credentials path", func(t *testing.T) {
		_, err := OAuth2("https://issuer", "audience", "")
		require.ErrorIs(t, err, ErrEmptyCredentialsPath)
	})

	t.Run("whitespace only issuer URL", func(t *testing.T) {
		_, err := OAuth2("   ", "audience", "/path")
		require.ErrorIs(t, err, ErrEmptyIssuerURL)
	})
}

// =============================================================================
// Athenz 测试
// =============================================================================

func TestAthenz(t *testing.T) {
	t.Parallel()

	t.Run("valid params", func(t *testing.T) {
		method, err := Athenz(map[string]string{
			"tenantDomain":  "my.domain",
			"tenantService": "my-service",
		})
		require.NoError(t, err)
		assert.NotNil(t, method.auth)
	})

	t.Run("nil params", func(t *testing.T) {
		_, err := Athenz(nil)
		require.ErrorIs(t, err, ErrEmptyAuthParams)
	})

	t.Run("empty map", func(t *testing.T) {
		_, err := Athenz(map[string]string{})
		require.ErrorIs(t, err, ErrEmptyAuthParams)
	})
}

// =============================================================================
// AuthMethod 通用测试
// =============================================================================

func TestAuthMethod_IsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, AuthMethod{}.IsZero())

	method, err := Token("test")
	require.NoError(t, err)
	assert.False(t, method.IsZero())
}

func TestAuthMethod_Authentication(t *testing.T) {
	t.Parallel()

	method, err := Token("test")
	require.NoError(t, err)
	assert.NotNil(t, method.Authentication())
}

// =============================================================================
// WithAuth Option 测试
// =============================================================================

func TestWithAuth(t *testing.T) {
	t.Parallel()

	t.Run("applies auth to options", func(t *testing.T) {
		method, err := Token("test-token")
		require.NoError(t, err)

		opts := defaultOptions()
		WithAuth(method)(opts)
		assert.NotNil(t, opts.Authentication)
	})

	t.Run("zero AuthMethod is no-op", func(t *testing.T) {
		opts := defaultOptions()
		WithAuth(AuthMethod{})(opts)
		assert.Nil(t, opts.Authentication)
	})
}
