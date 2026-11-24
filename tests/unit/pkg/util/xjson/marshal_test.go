package xjson

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testUser 用于测试的用户结构体，避免在多个测试函数中重复定义。
type testUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type panicMarshaler struct{}

func (panicMarshaler) MarshalJSON() ([]byte, error) {
	panic("MarshalJSON panic")
}

var errPanicSentinel = errors.New("sentinel from panic")

type panicErrorMarshaler struct{}

func (panicErrorMarshaler) MarshalJSON() ([]byte, error) {
	panic(errPanicSentinel)
}

func TestPrettyE(t *testing.T) {

	tests := []struct {
		name     string
		input    any
		contains string // 用于子串匹配（exact 为空时生效）
		exact    string // 精确匹配
		wantErr  bool
	}{
		{
			name:     "struct",
			input:    testUser{Name: "Alice", Age: 30},
			contains: `"name": "Alice"`,
		},
		{
			name:     "map",
			input:    map[string]int{"a": 1},
			contains: `"a": 1`,
		},
		{
			name:  "nil",
			input: nil,
			exact: "null",
		},
		{
			name:  "slice",
			input: []int{1, 2, 3},
			exact: "[\n  1,\n  2,\n  3\n]",
		},
		{
			name:  "empty_struct",
			input: struct{}{},
			exact: "{}",
		},
		{
			name:  "empty_string",
			input: "",
			exact: `""`,
		},
		{
			name: "nested_struct",
			input: struct {
				Outer string `json:"outer"`
				Inner struct {
					Value int `json:"value"`
				} `json:"inner"`
			}{Outer: "a", Inner: struct {
				Value int `json:"value"`
			}{Value: 1}},
			contains: `"inner": {`,
		},
		{
			name:  "html_special_chars",
			input: "<script>alert('xss')</script> & foo > bar",
			exact: `"\u003cscript\u003ealert('xss')\u003c/script\u003e \u0026 foo \u003e bar"`,
		},
		{
			name:    "error_NaN",
			input:   math.NaN(),
			wantErr: true,
		},
		{
			name:    "error_channel",
			input:   make(chan int),
			wantErr: true,
		},
		{
			name:    "error_panic_marshal",
			input:   panicMarshaler{},
			wantErr: true,
		},
		{
			name:    "error_panic_error_marshal",
			input:   panicErrorMarshaler{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrettyE(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				assert.True(t, errors.Is(err, ErrMarshal), "error should wrap ErrMarshal")
				return
			}
			require.NoError(t, err)
			if tt.exact != "" {
				assert.Equal(t, tt.exact, got)
			} else {
				assert.Contains(t, got, tt.contains)
			}
		})
	}
}

func TestPrettyE_PanicErrorChain(t *testing.T) {
	_, err := PrettyE(panicErrorMarshaler{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMarshal), "should wrap ErrMarshal")
	assert.True(t, errors.Is(err, errPanicSentinel), "should preserve panic error chain")
}

func TestPretty(t *testing.T) {
	// Pretty 的成功路径由 PrettyE 实现，TestPrettyE 已充分覆盖。
	// 此处仅保留代表性成功 case 验证委托正确性，重点测试错误降级行为。
	tests := []struct {
		name     string
		input    any
		contains string // 用于子串匹配（exact 为空时生效）
		exact    string // 精确匹配
	}{
		{
			name:     "struct",
			input:    testUser{Name: "Alice", Age: 30},
			contains: `"name": "Alice"`,
		},
		{
			name:  "nil",
			input: nil,
			exact: "null",
		},
		{
			name:     "error_NaN",
			input:    math.NaN(),
			contains: "<marshal error:",
		},
		{
			name:     "error_channel",
			input:    make(chan int),
			contains: "<marshal error:",
		},
		{
			name:     "error_panic_marshal",
			input:    panicMarshaler{},
			contains: "<marshal error:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pretty(tt.input)
			if tt.exact != "" {
				assert.Equal(t, tt.exact, got)
			} else {
				assert.Contains(t, got, tt.contains)
			}
		})
	}
}
