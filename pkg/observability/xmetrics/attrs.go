package xmetrics

import "time"

// String 创建字符串属性。
func String(key, value string) Attr {
	return Attr{Key: key, Value: value}
}

// Bool 创建布尔属性。
func Bool(key string, value bool) Attr {
	return Attr{Key: key, Value: value}
}

// Int 创建整数属性。
func Int(key string, value int) Attr {
	return Attr{Key: key, Value: value}
}

// Int64 创建 int64 属性。
func Int64(key string, value int64) Attr {
	return Attr{Key: key, Value: value}
}

// Uint64 创建 uint64 属性。
func Uint64(key string, value uint64) Attr {
	return Attr{Key: key, Value: value}
}

// Float64 创建 float64 属性。
func Float64(key string, value float64) Attr {
	return Attr{Key: key, Value: value}
}

// Duration 创建时间间隔属性。
// 建议显式使用带单位的 key，例如 "duration_ms"。
func Duration(key string, value time.Duration) Attr {
	return Attr{Key: key, Value: value}
}

// Any 创建任意类型属性。
func Any(key string, value any) Attr {
	return Attr{Key: key, Value: value}
}
