package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tt.want, tt.d.String())
		})
	}
}

func TestType_IsLocal(t *testing.T) {
	assert.True(t, Local.IsLocal())
	assert.False(t, SaaS.IsLocal())
	assert.False(t, Type("OTHER").IsLocal())
}

func TestType_IsSaaS(t *testing.T) {
	assert.True(t, SaaS.IsSaaS())
	assert.False(t, Local.IsSaaS())
	assert.False(t, Type("OTHER").IsSaaS())
}

func TestType_IsValid(t *testing.T) {
	assert.True(t, Local.IsValid())
	assert.True(t, SaaS.IsValid())
	assert.False(t, Type("").IsValid())
	assert.False(t, Type("OTHER").IsValid())
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
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParse_InvalidTypeIncludesInput(t *testing.T) {
	_, err := Parse("STAGING")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STAGING")
	assert.ErrorIs(t, err, ErrInvalidType)
}
