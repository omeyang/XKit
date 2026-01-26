package xetcd

import (
	"context"
	"crypto/tls"
	"testing"
	"time"
)

// testContextKey 用于测试的 context key 类型
type testContextKey string

func TestWithContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("test"), "value")

	o := defaultOptions()
	WithContext(ctx)(o)

	if o.ctx != ctx {
		t.Error("context was not set")
	}
}

func TestWithContext_Nil(t *testing.T) {
	var nilCtx context.Context
	o := defaultOptions()
	original := o.ctx

	WithContext(nilCtx)(o)

	if o.ctx != original {
		t.Error("nil context should not change the option")
	}
}

func TestWithHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		timeout        time.Duration
		wantEnabled    bool
		wantTimeout    time.Duration
		wantUseDefault bool
	}{
		{
			name:        "enabled with timeout",
			enabled:     true,
			timeout:     5 * time.Second,
			wantEnabled: true,
			wantTimeout: 5 * time.Second,
		},
		{
			name:           "enabled with zero timeout uses default",
			enabled:        true,
			timeout:        0,
			wantEnabled:    true,
			wantUseDefault: true,
		},
		{
			name:        "disabled",
			enabled:     false,
			timeout:     5 * time.Second,
			wantEnabled: false,
			wantTimeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := defaultOptions()
			defaultTimeout := o.healthTimeout

			WithHealthCheck(tt.enabled, tt.timeout)(o)

			if o.healthCheck != tt.wantEnabled {
				t.Errorf("healthCheck = %v, want %v", o.healthCheck, tt.wantEnabled)
			}

			if tt.wantUseDefault {
				if o.healthTimeout != defaultTimeout {
					t.Errorf("healthTimeout = %v, want default %v", o.healthTimeout, defaultTimeout)
				}
			} else {
				if o.healthTimeout != tt.wantTimeout {
					t.Errorf("healthTimeout = %v, want %v", o.healthTimeout, tt.wantTimeout)
				}
			}
		})
	}
}

func TestWithTLS(t *testing.T) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // G402: 测试代码中故意使用
	}

	o := defaultOptions()
	WithTLS(tlsConfig)(o)

	if o.tlsConfig != tlsConfig {
		t.Error("TLS config was not set")
	}
}

func TestWithTLS_Nil(t *testing.T) {
	o := defaultOptions()
	WithTLS(nil)(o)

	if o.tlsConfig != nil {
		t.Error("nil TLS config should set tlsConfig to nil")
	}
}

func TestDefaultOptions(t *testing.T) {
	o := defaultOptions()

	if o.ctx == nil {
		t.Error("ctx should not be nil")
	}
	if o.healthCheck {
		t.Error("healthCheck should be false by default")
	}
	if o.healthTimeout != 10*time.Second {
		t.Errorf("healthTimeout = %v, want %v", o.healthTimeout, 10*time.Second)
	}
	if o.tlsConfig != nil {
		t.Error("tlsConfig should be nil by default")
	}
}
