package xsys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetFileLimit_ZeroValue(t *testing.T) {
	// 参数校验在所有平台上行为一致。
	err := SetFileLimit(0)
	require.ErrorIs(t, err, ErrInvalidFileLimit)
}
