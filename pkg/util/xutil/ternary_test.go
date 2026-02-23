package xutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIf(t *testing.T) {
	t.Parallel()

	t.Run("int", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 1, If(true, 1, 2))
		assert.Equal(t, 2, If(false, 1, 2))
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "yes", If(true, "yes", "no"))
		assert.Equal(t, "no", If(false, "yes", "no"))
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()
		a, b := "a", "b"
		assert.Same(t, &a, If(true, &a, &b))
		assert.Same(t, &b, If(false, &a, &b))
	})

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		type S struct{ V int }
		assert.Equal(t, S{1}, If(true, S{1}, S{2}))
		assert.Equal(t, S{2}, If(false, S{1}, S{2}))
	})

	t.Run("interface", func(t *testing.T) {
		t.Parallel()
		var x, y any
		x = "hello"
		assert.Equal(t, x, If(true, x, y))
		assert.Equal(t, y, If(false, x, y))
	})

	t.Run("zero_values", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 42, If(true, 42, 0))
		assert.Equal(t, 0, If(false, 42, 0))
		assert.Equal(t, "", If(false, "hello", ""))
		assert.Equal(t, "default", If(false, "", "default"))
	})

	t.Run("nil_values", func(t *testing.T) {
		t.Parallel()
		// nil slice
		var nilSlice []int
		nonNilSlice := []int{1, 2, 3}
		assert.Nil(t, If(true, nilSlice, nonNilSlice))
		assert.Equal(t, nonNilSlice, If(false, nilSlice, nonNilSlice))
		assert.Equal(t, nonNilSlice, If(true, nonNilSlice, nilSlice))
		assert.Nil(t, If(false, nonNilSlice, nilSlice))

		// nil map
		var nilMap map[string]int
		nonNilMap := map[string]int{"a": 1}
		assert.Nil(t, If(true, nilMap, nonNilMap))
		assert.Equal(t, nonNilMap, If(false, nilMap, nonNilMap))

		// nil pointer
		type S struct{ V int }
		var nilPtr *S
		nonNilPtr := &S{V: 42}
		assert.Nil(t, If(true, nilPtr, nonNilPtr))
		assert.Equal(t, nonNilPtr, If(false, nilPtr, nonNilPtr))
		assert.Equal(t, nonNilPtr, If(true, nonNilPtr, nilPtr))
		assert.Nil(t, If(false, nonNilPtr, nilPtr))
	})

	t.Run("eager_evaluation", func(t *testing.T) {
		t.Parallel()
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
