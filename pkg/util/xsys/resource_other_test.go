//go:build !unix

package xsys

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetFileLimit_UnsupportedPlatform(t *testing.T) {
	err := SetFileLimit(1024)
	require.ErrorIs(t, err, ErrUnsupportedPlatform)
}

func TestGetFileLimit_UnsupportedPlatform(t *testing.T) {
	soft, hard, err := GetFileLimit()
	require.ErrorIs(t, err, ErrUnsupportedPlatform)
	assert.Equal(t, uint64(0), soft)
	assert.Equal(t, uint64(0), hard)
}
