package xauth

import (
	"os"
	"testing"
	"time"
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

		if cfg.Timeout != DefaultTimeout {
			t.Errorf("Timeout = %v, expected %v", cfg.Timeout, DefaultTimeout)
		}
		if cfg.TokenRefreshThreshold != DefaultTokenRefreshThreshold {
			t.Errorf("TokenRefreshThreshold = %v, expected %v", cfg.TokenRefreshThreshold, DefaultTokenRefreshThreshold)
		}
		if cfg.PlatformDataCacheTTL != DefaultPlatformDataCacheTTL {
			t.Errorf("PlatformDataCacheTTL = %v, expected %v", cfg.PlatformDataCacheTTL, DefaultPlatformDataCacheTTL)
		}
		// ClientID should be set based on environment
		if cfg.ClientID == "" {
			t.Error("ClientID should not be empty")
		}
		// ClientSecret defaults to ClientID
		if cfg.ClientSecret != cfg.ClientID {
			t.Errorf("ClientSecret = %v, expected %v", cfg.ClientSecret, cfg.ClientID)
		}
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

		if cfg.Timeout != 30*time.Second {
			t.Errorf("Timeout was overwritten")
		}
		if cfg.TokenRefreshThreshold != 10*time.Minute {
			t.Errorf("TokenRefreshThreshold was overwritten")
		}
		if cfg.PlatformDataCacheTTL != 1*time.Hour {
			t.Errorf("PlatformDataCacheTTL was overwritten")
		}
		if cfg.ClientID != "custom-client" {
			t.Errorf("ClientID was overwritten")
		}
		if cfg.ClientSecret != "custom-secret" {
			t.Errorf("ClientSecret was overwritten")
		}
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
