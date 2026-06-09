// Package xtls 提供通用的 mTLS 凭据构建工具，供各业务服务构建 gRPC/HTTP 服务器与客户端的
// transport credentials。本包只负责消费证书（从文件或 PEM 字节），不负责证书的签发、持久化
// 与轮转——这些交由各业务仓库自行处理，以保持本包的职责纯粹。
package xtls

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Config 描述 mTLS 证书来源与行为开关。
//
// 每一份证书（自身证书、私钥、CA）都支持两种供给方式：
//   - 文件路径：CertFile / KeyFile / CAFile
//   - PEM 字节：CertPEM / KeyPEM / CAPEM
//
// 同一字段两种来源同时给出时，优先使用 PEM 字节——便于通过配置中心或环境变量注入。
//
// 服务端与客户端共享同一 Config 类型，只是字段语义略有不同：
//   - 作为服务端：Cert* 是服务端证书，CA* 用于校验客户端证书（仅 RequireClientCert=true 时加载）
//   - 作为客户端：Cert* 是客户端证书（仅 mTLS 需要），CA* 用于校验服务端证书（留空则使用系统 CA 根）
type Config struct {
	// Enabled 总开关。设为 false 时 ServerCredentials / ClientCredentials 返回 insecure
	// 凭据，调用方可无条件将其传给 grpc.Creds / grpc.WithTransportCredentials。
	Enabled bool

	// CertFile 自身证书文件路径。
	CertFile string
	// KeyFile 自身私钥文件路径。
	KeyFile string
	// CertPEM 自身证书的 PEM 字节（与 CertFile 二选一，优先于 CertFile）。
	CertPEM []byte
	// KeyPEM 自身私钥的 PEM 字节（与 KeyFile 二选一，优先于 KeyFile）。
	KeyPEM []byte

	// CAFile 校验对端的 CA 证书文件路径。
	CAFile string
	// CAPEM 校验对端的 CA 证书 PEM 字节（与 CAFile 二选一，优先于 CAFile）。
	CAPEM []byte

	// ServerName 客户端向服务端发起连接时用于 SNI 与证书主机名校验。
	// 建议与服务端证书 DNSNames 中的名称一致；未设置时使用拨号地址的主机名。
	ServerName string

	// RequireClientCert 仅服务端生效。true 时强制 mTLS（双向认证），
	// 对应 tls.RequireAndVerifyClientCert，且必须同时提供 CAPEM/CAFile；
	// false 时仅单向 TLS（tls.NoClientCert），即使填了 CA 也不会校验客户端证书。
	// 默认 false（Go bool 零值）。
	RequireClientCert bool

	// MinVersion TLS 最低版本。零值时默认 tls.VersionTLS12。
	// 出于安全基线要求，低于 TLS 1.2 的取值会被强制抬升到 TLS 1.2。
	MinVersion uint16
}

// 常见错误，调用方可用 errors.Is 识别。
var (
	ErrMissingCert = errors.New("xtls: missing cert material (CertFile/CertPEM)")
	ErrMissingKey  = errors.New("xtls: missing key material (KeyFile/KeyPEM)")
	ErrMissingCA   = errors.New("xtls: missing CA material (CAFile/CAPEM)")
	ErrInvalidCA   = errors.New("xtls: invalid CA PEM")
)

// BuildServerTLSConfig 构建用于 TLS 服务端的 *tls.Config。
// Enabled=false 时返回 (nil, nil)，调用方据此决定是否启用 TLS。
func BuildServerTLSConfig(c Config) (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	cert, err := loadKeyPair(c)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if c.MinVersion > tls.VersionTLS12 {
		cfg.MinVersion = c.MinVersion
	}

	// 仅当显式要求 mTLS 时才加载 CA 并开启客户端证书校验；否则一律单向 TLS。
	cfg.ClientAuth = tls.NoClientCert
	if c.RequireClientCert {
		pool, err := loadCAPool(c)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

// BuildClientTLSConfig 构建用于 TLS 客户端的 *tls.Config。
// Enabled=false 时返回 (nil, nil)。
func BuildClientTLSConfig(c Config) (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	cfg := &tls.Config{
		ServerName: c.ServerName,
		MinVersion: tls.VersionTLS12,
	}
	if c.MinVersion > tls.VersionTLS12 {
		cfg.MinVersion = c.MinVersion
	}

	// 客户端证书仅在 mTLS 场景下需要；若未提供则走单向 TLS。
	if hasCertMaterial(c) || hasKeyMaterial(c) {
		cert, err := loadKeyPair(c)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	// CA 可选：未显式提供时 RootCAs 保持为 nil，由 crypto/tls 回退到系统 CA 根。
	// 适用于连接公网 HTTPS 服务等无需自定义 CA 的场景。
	if hasCAMaterial(c) {
		pool, err := loadCAPool(c)
		if err != nil {
			return nil, err
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

// ServerCredentials 返回 gRPC 服务端 TransportCredentials。
// Enabled=false 时返回 insecure 凭据，调用方可无条件使用。
func ServerCredentials(c Config) (credentials.TransportCredentials, error) {
	cfg, err := BuildServerTLSConfig(c)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(cfg), nil
}

// ClientCredentials 返回 gRPC 客户端 TransportCredentials。
// Enabled=false 时返回 insecure 凭据，调用方可无条件使用。
func ClientCredentials(c Config) (credentials.TransportCredentials, error) {
	cfg, err := BuildClientTLSConfig(c)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(cfg), nil
}

func loadKeyPair(c Config) (tls.Certificate, error) {
	certPEM, err := readMaterial(c.CertPEM, c.CertFile, ErrMissingCert)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := readMaterial(c.KeyPEM, c.KeyFile, ErrMissingKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("xtls: parse key pair: %w", err)
	}
	return cert, nil
}

func loadCAPool(c Config) (*x509.CertPool, error) {
	caPEM, err := readMaterial(c.CAPEM, c.CAFile, ErrMissingCA)
	if err != nil {
		return nil, err
	}
	// 先排除空白文件，避免把"文件存在但内容为空"误报为格式错，丢失诊断信息。
	if len(bytes.TrimSpace(caPEM)) == 0 {
		return nil, ErrMissingCA
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, ErrInvalidCA
	}
	return pool, nil
}

func readMaterial(inline []byte, path string, missingErr error) ([]byte, error) {
	if len(inline) > 0 {
		return inline, nil
	}
	if path == "" {
		return nil, missingErr
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("xtls: read %s: %w", path, err)
	}
	return data, nil
}

func hasCertMaterial(c Config) bool { return len(c.CertPEM) > 0 || c.CertFile != "" }
func hasKeyMaterial(c Config) bool  { return len(c.KeyPEM) > 0 || c.KeyFile != "" }
func hasCAMaterial(c Config) bool   { return len(c.CAPEM) > 0 || c.CAFile != "" }
