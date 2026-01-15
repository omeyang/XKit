package xmetrics

import (
	"context"
	"testing"
)

// FuzzStringAttr 模糊测试字符串属性创建
func FuzzStringAttr(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("key with spaces", "value with\nnewlines")
	f.Add("key\x00null", "value\x00null")
	f.Add("unicode键", "unicode值🎉")

	f.Fuzz(func(t *testing.T, key, value string) {
		attr := String(key, value)
		if attr.Key != key {
			t.Errorf("Key mismatch: got %q, want %q", attr.Key, key)
		}
		if attr.Value != value {
			t.Errorf("Value mismatch: got %q, want %q", attr.Value, value)
		}
	})
}

// FuzzIntAttr 模糊测试整数属性创建
func FuzzIntAttr(f *testing.F) {
	f.Add("count", 0)
	f.Add("count", 42)
	f.Add("count", -1)
	f.Add("count", 1<<30)
	f.Add("count", -(1 << 30))

	f.Fuzz(func(t *testing.T, key string, value int) {
		attr := Int(key, value)
		if attr.Key != key {
			t.Errorf("Key mismatch")
		}
		if attr.Value != value {
			t.Errorf("Value mismatch: got %v, want %d", attr.Value, value)
		}
	})
}

// FuzzInt64Attr 模糊测试 int64 属性创建
func FuzzInt64Attr(f *testing.F) {
	f.Add("count", int64(0))
	f.Add("count", int64(42))
	f.Add("count", int64(-1))
	f.Add("count", int64(1<<62))
	f.Add("count", int64(-(1 << 62)))

	f.Fuzz(func(t *testing.T, key string, value int64) {
		attr := Int64(key, value)
		if attr.Key != key {
			t.Errorf("Key mismatch")
		}
		if attr.Value != value {
			t.Errorf("Value mismatch")
		}
	})
}

// FuzzFloat64Attr 模糊测试浮点属性创建
func FuzzFloat64Attr(f *testing.F) {
	f.Add("ratio", 0.0)
	f.Add("ratio", 3.14159)
	f.Add("ratio", -1.0)
	f.Add("ratio", 1e308)
	f.Add("ratio", -1e308)

	f.Fuzz(func(t *testing.T, key string, value float64) {
		attr := Float64(key, value)
		if attr.Key != key {
			t.Errorf("Key mismatch")
		}
		// 浮点比较需要特殊处理 NaN
		if v, ok := attr.Value.(float64); ok {
			// 两个 NaN 比较总是 false，所以用 IsNaN 检查
			if value != v && !(value != value && v != v) {
				t.Errorf("Value mismatch")
			}
		}
	})
}

// FuzzBoolAttr 模糊测试布尔属性创建
func FuzzBoolAttr(f *testing.F) {
	f.Add("enabled", true)
	f.Add("enabled", false)

	f.Fuzz(func(t *testing.T, key string, value bool) {
		attr := Bool(key, value)
		if attr.Key != key {
			t.Errorf("Key mismatch")
		}
		if attr.Value != value {
			t.Errorf("Value mismatch")
		}
	})
}

// FuzzSpanOptions 模糊测试 SpanOptions 创建
func FuzzSpanOptions(f *testing.F) {
	f.Add("component", "operation", uint8(0))
	f.Add("http", "GET /api", uint8(1))
	f.Add("db", "SELECT", uint8(2))
	f.Add("", "", uint8(5))

	f.Fuzz(func(t *testing.T, component, operation string, kind uint8) {
		// 将 kind 映射到有效的 Kind 值
		mappedKind := Kind(kind % 5)

		opts := SpanOptions{
			Component: component,
			Operation: operation,
			Kind:      mappedKind,
		}

		if opts.Component != component {
			t.Errorf("Component mismatch")
		}
		if opts.Operation != operation {
			t.Errorf("Operation mismatch")
		}
	})
}

// FuzzNoopObserver 模糊测试 NoopObserver
func FuzzNoopObserver(f *testing.F) {
	f.Add("component", "operation")
	f.Add("", "")
	f.Add("test\x00null", "test\nnewline")

	f.Fuzz(func(t *testing.T, component, operation string) {
		observer := NoopObserver{}
		ctx := context.Background()
		opts := SpanOptions{
			Component: component,
			Operation: operation,
			Kind:      KindServer,
		}

		newCtx, span := observer.Start(ctx, opts)

		// NoopObserver 应该返回原始 context
		if newCtx != ctx {
			t.Errorf("Context should be unchanged")
		}

		// span 应该是 NoopSpan
		if _, ok := span.(NoopSpan); !ok {
			t.Errorf("Expected NoopSpan")
		}

		// End 不应 panic
		span.End(Result{Status: StatusOK})
		span.End(Result{Status: StatusError})
	})
}

// FuzzStart 模糊测试 Start 辅助函数
func FuzzStart(f *testing.F) {
	f.Add("component", "operation", true)
	f.Add("", "", false)

	f.Fuzz(func(t *testing.T, component, operation string, useObserver bool) {
		ctx := context.Background()
		opts := SpanOptions{
			Component: component,
			Operation: operation,
		}

		var observer Observer
		if useObserver {
			observer = NoopObserver{}
		}

		newCtx, span := Start(ctx, observer, opts)

		// 验证返回值不为 nil
		if newCtx == nil {
			t.Error("Context should not be nil")
		}
		if span == nil {
			t.Error("Span should not be nil")
		}

		// End 不应 panic
		span.End(Result{})
	})
}

// FuzzResult 模糊测试 Result 结构
func FuzzResult(f *testing.F) {
	f.Add(uint8(0), "")
	f.Add(uint8(1), "error message")
	f.Add(uint8(2), "unicode错误🚫")

	f.Fuzz(func(t *testing.T, status uint8, errMsg string) {
		mappedStatus := Status(status % 2)

		result := Result{
			Status: mappedStatus,
		}

		if errMsg != "" {
			result.Err = errWrapper{msg: errMsg}
		}

		// 验证结构
		if result.Status != mappedStatus {
			t.Error("Status mismatch")
		}

		// NoopSpan.End 不应 panic
		span := NoopSpan{}
		span.End(result)
	})
}

// errWrapper 用于模糊测试的错误包装
type errWrapper struct {
	msg string
}

func (e errWrapper) Error() string {
	return e.msg
}
