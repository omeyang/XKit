package xutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIf(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		assert.Equal(t, 1, If(true, 1, 2))
		assert.Equal(t, 2, If(false, 1, 2))
	})

	t.Run("string", func(t *testing.T) {
		assert.Equal(t, "yes", If(true, "yes", "no"))
		assert.Equal(t, "no", If(false, "yes", "no"))
	})

	t.Run("pointer", func(t *testing.T) {
		a, b := "a", "b"
		assert.Equal(t, &a, If(true, &a, &b))
		assert.Equal(t, &b, If(false, &a, &b))
	})

	t.Run("struct", func(t *testing.T) {
		type S struct{ V int }
		assert.Equal(t, S{1}, If(true, S{1}, S{2}))
		assert.Equal(t, S{2}, If(false, S{1}, S{2}))
	})

	t.Run("interface", func(t *testing.T) {
		var x, y any
		x = "hello"
		assert.Equal(t, x, If(true, x, y))
		assert.Equal(t, y, If(false, x, y))
	})

	t.Run("zero_values", func(t *testing.T) {
		assert.Equal(t, 42, If(true, 42, 0))
		assert.Equal(t, 0, If(false, 42, 0))
		assert.Equal(t, "", If(false, "hello", ""))
		assert.Equal(t, "default", If(false, "", "default"))
	})

	t.Run("eager_evaluation", func(t *testing.T) {
		// 验证两个分支参数均被求值（eager semantics），
		// 防止未来重构意外引入惰性求值。
		trueEval, falseEval := 0, 0
		evalTrue := func() int { trueEval++; return 1 }
		evalFalse := func() int { falseEval++; return 2 }

		result := If(true, evalTrue(), evalFalse())
		assert.Equal(t, 1, result)
		assert.Equal(t, 1, trueEval, "trueVal 应被求值")
		assert.Equal(t, 1, falseEval, "falseVal 也应被求值（eager）")

		trueEval, falseEval = 0, 0
		result = If(false, evalTrue(), evalFalse())
		assert.Equal(t, 2, result)
		assert.Equal(t, 1, trueEval, "trueVal 应被求值（eager）")
		assert.Equal(t, 1, falseEval, "falseVal 应被求值")
	})
}
