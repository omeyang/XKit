//go:build unix && !freebsd && !dragonfly

package xsys

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rlimit 字段为 uint64 的平台：转换无损，无溢出分支。
func TestRlimConversion_Uint64Platform(t *testing.T) {
	t.Parallel()

	cases := []uint64{1, 1024, math.MaxInt64, math.MaxUint64}
	for _, v := range cases {
		got, err := rlimFromUint64(v)
		require.NoError(t, err)
		assert.Equal(t, v, rlimToUint64(got))
	}
}
