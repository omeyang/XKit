package xpulsar

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/apache/pulsar-client-go/pulsar"
)

// =============================================================================
// AuthMethod 类型
// =============================================================================

// AuthMethod 封装 Pulsar 认证方式。
// 通过工厂函数（Token、TLSCert、OAuth2 等）创建，传入 WithAuth 配置客户端。
//
// 用法:
//
//	auth, err := xpulsar.Token("my-token")
//	if err != nil {
//	    return err
//	}
//	client, err := xpulsar.NewClient("pulsar://localhost:6650",
//	    xpulsar.WithAuth(auth),
//	)
type AuthMethod struct {
	auth pulsar.Authentication
}

// Authentication 返回底层的 pulsar.Authentication。
// 用于需要直接操作原生认证对象的场景。
func (m AuthMethod) Authentication() pulsar.Authentication {
	return m.auth
}

// IsZero 返回 AuthMethod 是否为零值（未配置认证）。
func (m AuthMethod) IsZero() bool {
	return m.auth == nil
}

// =============================================================================
// 工厂函数
// =============================================================================

// Token 创建基于静态 token 的认证。
// token 不能为空或仅含空白字符。前后空白会被去除后传递给 SDK，
// 避免复制粘贴引入的空白在运行时以认证失败形式暴露。
func Token(token string) (AuthMethod, error) {
	if token = strings.TrimSpace(token); token == "" {
		return AuthMethod{}, ErrEmptyToken
	}
	return AuthMethod{auth: pulsar.NewAuthenticationToken(token)}, nil
}

// TokenFromFile 创建基于文件的 token 认证。
// Pulsar SDK 在需要时从文件读取 token，支持 token 轮换。
// 前后空白会被去除后传递给 SDK，避免因路径含空白导致文件打开失败。
func TokenFromFile(path string) (AuthMethod, error) {
	if path = strings.TrimSpace(path); path == "" {
		return AuthMethod{}, ErrEmptyTokenFilePath
	}
	return AuthMethod{auth: pulsar.NewAuthenticationTokenFromFile(path)}, nil
}

// TokenFromEnv 创建基于环境变量的 token 认证。
// envKey 为环境变量名，在创建时立即读取（非延迟读取）。
// envKey 前后空白会被去除，确保 os.Getenv 按实际 key 查找。
//
// 注意: 与 Token 不同，TokenFromEnv 不校验环境变量的值是否为空——
// 环境变量可能在部署环境中已设置但本地开发环境中不存在。
func TokenFromEnv(envKey string) (AuthMethod, error) {
	if envKey = strings.TrimSpace(envKey); envKey == "" {
		return AuthMethod{}, ErrEmptyEnvKey
	}
	token := os.Getenv(envKey)
	// 即使环境变量为空也创建认证对象——让 Pulsar SDK 返回更具体的错误，
	// 而非在这里阻止用户（环境变量可能在运行环境中已设置但本地未设）。
	return AuthMethod{auth: pulsar.NewAuthenticationToken(token)}, nil
}

// TokenFromSupplier 创建基于动态 supplier 的 token 认证。
// supplier 在每次认证时被调用，适合 token 需要动态刷新的场景。
func TokenFromSupplier(supplier func() (string, error)) (AuthMethod, error) {
	if supplier == nil {
		return AuthMethod{}, fmt.Errorf("%w: token supplier", ErrNilSupplier)
	}
	return AuthMethod{auth: pulsar.NewAuthenticationTokenFromSupplier(supplier)}, nil
}

// TLSCert 创建基于 mTLS 证书的认证。
// certPath/keyPath 前后空白会被去除后传递给 SDK，避免因路径含空白导致文件打开失败。
func TLSCert(certPath, keyPath string) (AuthMethod, error) {
	if certPath = strings.TrimSpace(certPath); certPath == "" {
		return AuthMethod{}, ErrEmptyCertPath
	}
	if keyPath = strings.TrimSpace(keyPath); keyPath == "" {
		return AuthMethod{}, ErrEmptyKeyPath
	}
	return AuthMethod{auth: pulsar.NewAuthenticationTLS(certPath, keyPath)}, nil
}

// TLSCertFromSupplier 创建基于动态证书 supplier 的 mTLS 认证。
// supplier 在每次 TLS 握手时被调用，适合证书需要动态轮换的场景。
func TLSCertFromSupplier(supplier func() (*tls.Certificate, error)) (AuthMethod, error) {
	if supplier == nil {
		return AuthMethod{}, fmt.Errorf("%w: TLS cert supplier", ErrNilSupplier)
	}
	return AuthMethod{auth: pulsar.NewAuthenticationFromTLSCertSupplier(supplier)}, nil
}

// OAuth2 创建 OAuth2 认证。
// 相比 middlewarex 的 map[string]string 参数，使用类型安全的参数。
// 所有字段前后空白会被去除后传递给 SDK，避免复制粘贴引入的空白在运行时暴露。
//
// 参数说明:
//   - issuerURL: OAuth2 Token 签发地址
//   - audience: 目标受众标识（通常为 Pulsar 集群标识）
//   - credentialsPath: 客户端凭证文件路径（JSON 格式，包含 client_id 和 client_secret）
func OAuth2(issuerURL, audience, credentialsPath string) (AuthMethod, error) {
	if issuerURL = strings.TrimSpace(issuerURL); issuerURL == "" {
		return AuthMethod{}, ErrEmptyIssuerURL
	}
	if audience = strings.TrimSpace(audience); audience == "" {
		return AuthMethod{}, ErrEmptyAudience
	}
	if credentialsPath = strings.TrimSpace(credentialsPath); credentialsPath == "" {
		return AuthMethod{}, ErrEmptyCredentialsPath
	}
	return AuthMethod{auth: pulsar.NewAuthenticationOAuth2(map[string]string{
		"type":              "client_credentials",
		"issuerUrl":         issuerURL,
		"audience":          audience,
		"privateKey":        credentialsPath,
		"clientCredentials": credentialsPath, // 部分版本 SDK 使用此 key
	})}, nil
}

// Athenz 创建 Athenz 认证。
// 保留 map[string]string 参数，因为 Athenz 参数较多且不固定。
//
// 典型参数: tenantDomain, tenantService, providerDomain, keyId, privateKey, principalHeader 等。
func Athenz(params map[string]string) (AuthMethod, error) {
	if len(params) == 0 {
		return AuthMethod{}, ErrEmptyAuthParams
	}
	return AuthMethod{auth: pulsar.NewAuthenticationAthenz(params)}, nil
}

// =============================================================================
// Option
// =============================================================================

// WithAuth 设置认证方式。
// 零值 AuthMethod（未配置认证）会被忽略。
//
// 这是 WithAuthentication 的高级替代，提供参数校验和类型安全。
// 如需直接传入 pulsar.Authentication，请使用 WithAuthentication。
func WithAuth(method AuthMethod) Option {
	return func(o *clientOptions) {
		if !method.IsZero() {
			o.Authentication = method.auth
		}
	}
}
