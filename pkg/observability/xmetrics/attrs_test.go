package xmetrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// 属性创建函数测试
// ============================================================================

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"normal", "key", "value"},
		{"empty_key", "", "value"},
		{"empty_value", "key", ""},
		{"both_empty", "", ""},
		{"unicode", "键", "值"},
		{"special_chars", "key-with-dots.and/slashes", "value with spaces"},
		{"long_value", "key", string(make([]byte, 10000))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := String(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value bool
	}{
		{"true", "enabled", true},
		{"false", "disabled", false},
		{"empty_key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Bool(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value int
	}{
		{"positive", "count", 42},
		{"negative", "offset", -100},
		{"zero", "index", 0},
		{"max_int32", "max32", 2147483647},
		{"min_int32", "min32", -2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Int(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value int64
	}{
		{"positive", "timestamp", 1704067200000},
		{"negative", "offset", -9223372036854775808},
		{"zero", "index", 0},
		{"max", "max", 9223372036854775807},
		{"min", "min", -9223372036854775808},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Int64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value uint64
	}{
		{"positive", "bytes", 1024},
		{"zero", "count", 0},
		{"max", "max", 18446744073709551615},
		{"large", "large", 10000000000000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Uint64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value float64
	}{
		{"positive", "ratio", 3.14159},
		{"negative", "offset", -2.71828},
		{"zero", "rate", 0.0},
		{"very_small", "epsilon", 1e-300},
		{"very_large", "big", 1e300},
		{"fraction", "half", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Float64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value time.Duration
	}{
		{"nanosecond", "latency_ns", time.Nanosecond},
		{"microsecond", "latency_us", time.Microsecond},
		{"millisecond", "latency_ms", time.Millisecond},
		{"second", "timeout", time.Second},
		{"minute", "interval", time.Minute},
		{"hour", "ttl", time.Hour},
		{"zero", "zero", 0},
		{"negative", "negative", -time.Second},
		{"composite", "composite", 2*time.Hour + 30*time.Minute + 15*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Duration(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestAny(t *testing.T) {
	t.Parallel()

	type customStruct struct {
		Name  string
		Value int
	}

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{"string", "str", "hello"},
		{"int", "num", 42},
		{"float", "ratio", 3.14},
		{"bool", "flag", true},
		{"nil", "empty", nil},
		{"slice", "list", []int{1, 2, 3}},
		{"map", "dict", map[string]int{"a": 1}},
		{"struct", "obj", customStruct{Name: "test", Value: 100}},
		{"pointer", "ptr", new(int)},
		{"channel", "ch", make(chan int)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Any(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			// 对于 channel 类型，只验证 key 和 value 不为 nil
			if tt.name == "channel" {
				assert.NotNil(t, attr.Value)
			} else {
				assert.Equal(t, tt.value, attr.Value)
			}
		})
	}

	// 单独测试 func 类型（不能用 Equal 比较）
	t.Run("func", func(t *testing.T) {
		fn := func() {}
		attr := Any("fn", fn)
		assert.Equal(t, "fn", attr.Key)
		assert.NotNil(t, attr.Value)
	})
}

// ============================================================================
// 类型一致性测试
// ============================================================================

func TestAttrFunctions_ReturnType(t *testing.T) {
	t.Parallel()

	// 验证所有函数返回 Attr 类型
	var attr Attr

	attr = String("k", "v")
	assert.IsType(t, Attr{}, attr)

	attr = Bool("k", true)
	assert.IsType(t, Attr{}, attr)

	attr = Int("k", 1)
	assert.IsType(t, Attr{}, attr)

	attr = Int64("k", 1)
	assert.IsType(t, Attr{}, attr)

	attr = Uint64("k", 1)
	assert.IsType(t, Attr{}, attr)

	attr = Float64("k", 1.0)
	assert.IsType(t, Attr{}, attr)

	attr = Duration("k", time.Second)
	assert.IsType(t, Attr{}, attr)

	attr = Any("k", nil)
	assert.IsType(t, Attr{}, attr)
}

// ============================================================================
// 组合使用测试
// ============================================================================

func TestAttrFunctions_InSlice(t *testing.T) {
	t.Parallel()

	attrs := []Attr{
		String("component", "xobs"),
		Int("version", 1),
		Bool("enabled", true),
		Float64("ratio", 0.95),
		Duration("timeout", 5*time.Second),
		Any("extra", map[string]string{"env": "test"}),
	}

	assert.Len(t, attrs, 6)

	// 验证各个属性
	assert.Equal(t, "component", attrs[0].Key)
	assert.Equal(t, "xobs", attrs[0].Value)

	assert.Equal(t, "version", attrs[1].Key)
	assert.Equal(t, 1, attrs[1].Value)

	assert.Equal(t, "enabled", attrs[2].Key)
	assert.Equal(t, true, attrs[2].Value)

	assert.Equal(t, "ratio", attrs[3].Key)
	assert.Equal(t, 0.95, attrs[3].Value)

	assert.Equal(t, "timeout", attrs[4].Key)
	assert.Equal(t, 5*time.Second, attrs[4].Value)

	assert.Equal(t, "extra", attrs[5].Key)
}

func TestAttrFunctions_InSpanOptions(t *testing.T) {
	t.Parallel()

	opts := SpanOptions{
		Component: "test",
		Operation: "attrs-test",
		Kind:      KindInternal,
		Attrs: []Attr{
			String("service", "my-service"),
			Int("port", 8080),
			Bool("tls", true),
		},
	}

	assert.Len(t, opts.Attrs, 3)
	assert.Equal(t, "service", opts.Attrs[0].Key)
	assert.Equal(t, "my-service", opts.Attrs[0].Value)
}

func TestAttrFunctions_InResult(t *testing.T) {
	t.Parallel()

	result := Result{
		Status: StatusOK,
		Attrs: []Attr{
			Int64("bytes_written", 1024),
			Duration("elapsed", 100*time.Millisecond),
			String("cache_status", "hit"),
		},
	}

	assert.Len(t, result.Attrs, 3)
	assert.Equal(t, "bytes_written", result.Attrs[0].Key)
	assert.Equal(t, int64(1024), result.Attrs[0].Value)
}

// ============================================================================
// 边界值测试
// ============================================================================

func TestAttrFunctions_EdgeCases(t *testing.T) {
	t.Parallel()

	// 空字符串
	attr := String("", "")
	assert.Empty(t, attr.Key)
	assert.Empty(t, attr.Value)

	// 零值 Duration
	attr = Duration("zero", 0)
	assert.Equal(t, time.Duration(0), attr.Value)

	// 最大/最小整数
	attr = Int64("max", 9223372036854775807)
	assert.Equal(t, int64(9223372036854775807), attr.Value)

	attr = Int64("min", -9223372036854775808)
	assert.Equal(t, int64(-9223372036854775808), attr.Value)

	// 最大 uint64
	attr = Uint64("max", 18446744073709551615)
	assert.Equal(t, uint64(18446744073709551615), attr.Value)

	// 特殊浮点数
	attr = Float64("inf", 1e308)
	assert.NotZero(t, attr.Value)
}
