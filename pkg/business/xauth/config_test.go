package xauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: ErrNilConfig,
		},
		{
			name:    "empty host",
			config:  &Config{},
			wantErr: ErrMissingHost,
		},
		{
			name:    "whitespace host",
			config:  &Config{Host: "   "},
			wantErr: ErrMissingHost,
		},
		{
			name:    "negative timeout",
			config:  &Config{Host: "https://auth.test.com", Timeout: -1},
			wantErr: ErrInvalidTimeout,
		},
		{
			name:    "negative refresh threshold",
			config:  &Config{Host: "https://auth.test.com", TokenRefreshThreshold: -1},
			wantErr: ErrInvalidRefreshThreshold,
		},
		{
			name:    "http host rejected by default",
			config:  &Config{Host: "http://auth.test.com"},
			wantErr: ErrInsecureHost,
		},
		{
			name:    "http host allowed with AllowInsecure",
			config:  &Config{Host: "http://auth.test.com", AllowInsecure: true},
			wantErr: nil,
		},
		{
			name:    "valid config",
			config:  &Config{Host: "https://auth.test.com"},
			wantErr: nil,
		},
		{
			name: "valid config with all fields",
			config: &Config{
				Host:                  "https://auth.test.com",
				ClientID:              "test-client",
				ClientSecret:          "test-secret",
				APIKey:                "test-api-key",
				Timeout:               30 * time.Second,
				TokenRefreshThreshold: 10 * time.Minute,
				PlatformDataCacheTTL:  1 * time.Hour,
			},
			wantErr: nil,
		},
		{
			name:    "host without scheme",
			config:  &Config{Host: "auth.test.com"},
			wantErr: ErrInvalidHost,
		},
		{
			name:    "host with unsupported scheme",
			config:  &Config{Host: "ftp://auth.test.com"},
			wantErr: ErrInvalidHost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	t.Run("apply all defaults", func(t *testing.T) {
		cfg := &Config{Host: "https://auth.test.com"}
		cfg.ApplyDefaults()

		assert.Equal(t, DefaultTimeout, cfg.Timeout)
		assert.Equal(t, DefaultTokenRefreshThreshold, cfg.TokenRefreshThreshold)
		assert.Equal(t, DefaultPlatformDataCacheTTL, cfg.PlatformDataCacheTTL)
		// ClientID should be set based on environment
		assert.NotEmpty(t, cfg.ClientID, "ClientID should not be empty")
		// ClientSecret defaults to ClientID
		assert.Equal(t, cfg.ClientID, cfg.ClientSecret)
	})

	t.Run("preserve existing values", func(t *testing.T) {
		cfg := &Config{
			Host:                  "https://auth.test.com",
			ClientID:              "custom-client",
			ClientSecret:          "custom-secret",
			Timeout:               30 * time.Second,
			TokenRefreshThreshold: 10 * time.Minute,
			PlatformDataCacheTTL:  1 * time.Hour,
		}
		cfg.ApplyDefaults()

		assert.Equal(t, 30*time.Second, cfg.Timeout, "Timeout was overwritten")
		assert.Equal(t, 10*time.Minute, cfg.TokenRefreshThreshold, "TokenRefreshThreshold was overwritten")
		assert.Equal(t, 1*time.Hour, cfg.PlatformDataCacheTTL, "PlatformDataCacheTTL was overwritten")
		assert.Equal(t, "custom-client", cfg.ClientID, "ClientID was overwritten")
		assert.Equal(t, "custom-secret", cfg.ClientSecret, "ClientSecret was overwritten")
	})
}

func TestConfig_Clone(t *testing.T) {
	t.Run("clone nil", func(t *testing.T) {
		var cfg *Config
		clone := cfg.Clone()
		if clone != nil {
			t.Error("Clone of nil should be nil")
		}
	})

	t.Run("clone basic config", func(t *testing.T) {
		cfg := &Config{
			Host:     "https://auth.test.com",
			ClientID: "test-client",
			Timeout:  15 * time.Second,
		}
		clone := cfg.Clone()

		// Verify values are copied
		if clone.Host != cfg.Host {
			t.Errorf("Host = %v, expected %v", clone.Host, cfg.Host)
		}
		if clone.ClientID != cfg.ClientID {
			t.Errorf("ClientID = %v, expected %v", clone.ClientID, cfg.ClientID)
		}

		// Modify original, verify clone is not affected
		cfg.Host = "https://other.test.com"
		if clone.Host == cfg.Host {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("clone with TLS config", func(t *testing.T) {
		cfg := &Config{
			Host: "https://auth.test.com",
			TLS: &TLSConfig{
				InsecureSkipVerify: true,
				RootCAFile:         "/path/to/ca.crt",
			},
		}
		clone := cfg.Clone()

		// Verify TLS config is copied
		if clone.TLS == nil {
			t.Fatal("TLS config should be cloned")
		}
		if clone.TLS.InsecureSkipVerify != cfg.TLS.InsecureSkipVerify {
			t.Errorf("TLS.InsecureSkipVerify = %v, expected %v", clone.TLS.InsecureSkipVerify, cfg.TLS.InsecureSkipVerify)
		}

		// Verify TLS config is independent
		if clone.TLS == cfg.TLS {
			t.Error("TLS config should be a separate copy")
		}
	})
}

func TestTLSConfig_BuildTLSConfig(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *TLSConfig
		tlsCfg, err := cfg.BuildTLSConfig()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if tlsCfg != nil {
			t.Error("expected nil tls.Config for nil TLSConfig")
		}
	})

	t.Run("insecure skip verify", func(t *testing.T) {
		cfg := &TLSConfig{InsecureSkipVerify: true}
		tlsCfg, err := cfg.BuildTLSConfig()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !tlsCfg.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be true")
		}
	})

	t.Run("invalid CA file", func(t *testing.T) {
		cfg := &TLSConfig{RootCAFile: "/nonexistent/ca.crt"}
		_, err := cfg.BuildTLSConfig()
		if err == nil {
			t.Error("expected error for nonexistent CA file")
		}
	})

	t.Run("invalid cert file", func(t *testing.T) {
		cfg := &TLSConfig{
			CertFile: "/nonexistent/cert.crt",
			KeyFile:  "/nonexistent/key.key",
		}
		_, err := cfg.BuildTLSConfig()
		if err == nil {
			t.Error("expected error for nonexistent cert file")
		}
	})

	t.Run("invalid CA PEM content", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "bad-ca.pem")
		if err := os.WriteFile(caFile, []byte("not-a-pem"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &TLSConfig{RootCAFile: caFile}
		_, err := cfg.BuildTLSConfig()
		if err == nil {
			t.Error("expected error for invalid CA PEM content")
		}
	})

	t.Run("valid CA cert", func(t *testing.T) {
		// 使用自签名 CA 证书测试成功路径
		caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		caTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
		}
		caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		caFile := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
			t.Fatal(err)
		}

		cfg := &TLSConfig{RootCAFile: caFile}
		tlsCfg, err := cfg.BuildTLSConfig()
		if err != nil {
			t.Fatalf("BuildTLSConfig failed: %v", err)
		}
		if tlsCfg.RootCAs == nil {
			t.Error("RootCAs should not be nil")
		}
	})

	t.Run("valid client cert", func(t *testing.T) {
		// 使用自签名证书测试客户端证书加载
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "Test Client"},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(time.Hour),
		}
		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}

		dir := t.TempDir()
		certFile := filepath.Join(dir, "cert.pem")
		keyFile := filepath.Join(dir, "key.pem")

		if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o600); err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
			t.Fatal(err)
		}

		cfg := &TLSConfig{CertFile: certFile, KeyFile: keyFile}
		tlsCfg, err := cfg.BuildTLSConfig()
		if err != nil {
			t.Fatalf("BuildTLSConfig failed: %v", err)
		}
		if len(tlsCfg.Certificates) != 1 {
			t.Errorf("Certificates count = %d, expected 1", len(tlsCfg.Certificates))
		}
	})
}

func TestIsLocalEnv(t *testing.T) {
	// Note: This test depends on environment variables
	// In real tests, you might want to mock os.Getenv
	result := IsLocalEnv()
	// Just verify it doesn't panic
	_ = result
}

func TestGetTenantIDFromEnv(t *testing.T) {
	// Note: This test depends on environment variables
	result := GetTenantIDFromEnv()
	// Just verify it doesn't panic
	_ = result
}

func TestGetDefaultClientID(t *testing.T) {
	// This depends on DEPLOYMENT_TYPE env var
	clientID := getDefaultClientID()
	if clientID != DefaultLocalClientID && clientID != DefaultSaaSClientID {
		t.Errorf("unexpected clientID: %s", clientID)
	}
}

func TestConfig_ApplyDefaults_ClientIDFromEnv(t *testing.T) {
	// Save and restore env
	oldType := os.Getenv(EnvKeyDeploymentType)
	defer os.Setenv(EnvKeyDeploymentType, oldType)

	t.Run("local deployment", func(t *testing.T) {
		os.Setenv(EnvKeyDeploymentType, DeploymentTypeLocal)
		cfg := &Config{Host: "https://auth.test.com"}
		cfg.ApplyDefaults()

		if cfg.ClientID != DefaultLocalClientID {
			t.Errorf("ClientID = %q, expected %q", cfg.ClientID, DefaultLocalClientID)
		}
	})

	t.Run("saas deployment", func(t *testing.T) {
		os.Setenv(EnvKeyDeploymentType, DeploymentTypeSaaS)
		cfg := &Config{Host: "https://auth.test.com"}
		cfg.ApplyDefaults()

		if cfg.ClientID != DefaultSaaSClientID {
			t.Errorf("ClientID = %q, expected %q", cfg.ClientID, DefaultSaaSClientID)
		}
	})
}

func TestTLSConfig_BuildTLSConfig_MinVersion(t *testing.T) {
	// BuildTLSConfig always sets MinVersion to TLS 1.2
	cfg := &TLSConfig{}
	tlsCfg, err := cfg.BuildTLSConfig()
	if err != nil {
		t.Fatalf("BuildTLSConfig failed: %v", err)
	}
	if tlsCfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %x, expected TLS 1.2 (0x0303)", tlsCfg.MinVersion)
	}
}
