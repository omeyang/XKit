package deploy

import (
	"errors"
	"strings"
	"testing"
)

func TestType_String(t *testing.T) {
	tests := []struct {
		name string
		d    Type
		want string
	}{
		{"Local", Local, "LOCAL"},
		{"SaaS", SaaS, "SAAS"},
		{"empty", Type(""), ""},
		{"custom", Type("CUSTOM"), "CUSTOM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Type.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestType_IsLocal(t *testing.T) {
	if !Local.IsLocal() {
		t.Error("Local.IsLocal() = false, want true")
	}
	if SaaS.IsLocal() {
		t.Error("SaaS.IsLocal() = true, want false")
	}
	if Type("OTHER").IsLocal() {
		t.Error(`Type("OTHER").IsLocal() = true, want false`)
	}
}

func TestType_IsSaaS(t *testing.T) {
	if !SaaS.IsSaaS() {
		t.Error("SaaS.IsSaaS() = false, want true")
	}
	if Local.IsSaaS() {
		t.Error("Local.IsSaaS() = true, want false")
	}
	if Type("OTHER").IsSaaS() {
		t.Error(`Type("OTHER").IsSaaS() = true, want false`)
	}
}

func TestType_IsValid(t *testing.T) {
	if !Local.IsValid() {
		t.Error("Local.IsValid() = false, want true")
	}
	if !SaaS.IsValid() {
		t.Error("SaaS.IsValid() = false, want true")
	}
	if Type("").IsValid() {
		t.Error(`Type("").IsValid() = true, want false`)
	}
	if Type("OTHER").IsValid() {
		t.Error(`Type("OTHER").IsValid() = true, want false`)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Type
		wantErr error
	}{
		{"LOCAL uppercase", "LOCAL", Local, nil},
		{"LOCAL lowercase", "local", Local, nil},
		{"LOCAL mixed case", "Local", Local, nil},
		{"LOCAL with spaces", "  LOCAL  ", Local, nil},
		{"SAAS uppercase", "SAAS", SaaS, nil},
		{"SAAS lowercase", "saas", SaaS, nil},
		{"SAAS mixed case", "SaaS", SaaS, nil},
		{"SAAS with spaces", "  SAAS  ", SaaS, nil},
		{"empty string", "", Type(""), ErrMissingValue},
		{"whitespace only", "   ", Type(""), ErrMissingValue},
		{"invalid value", "STAGING", Type(""), ErrInvalidType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Parse(%q) error = %v, want %v", tt.input, err, tt.wantErr)
				}
				if got != "" {
					t.Errorf("Parse(%q) = %q, want empty on error", tt.input, got)
				}
			} else {
				if err != nil {
					t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
				}
				if got != tt.want {
					t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestParse_InvalidTypeIncludesInput(t *testing.T) {
	_, err := Parse("STAGING")
	if err == nil {
		t.Fatal(`Parse("STAGING") error = nil, want error`)
	}
	if !errors.Is(err, ErrInvalidType) {
		t.Errorf(`Parse("STAGING") error = %v, want ErrInvalidType`, err)
	}
	if !strings.Contains(err.Error(), "STAGING") {
		t.Errorf(`Parse("STAGING") error = %q, should contain "STAGING"`, err)
	}
}
