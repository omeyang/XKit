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

// Float32 创建 float32 属性。
// OTel 转换时会提升为 float64。
func Float32(key string, value float32) Attr {
	return Attr{Key: key, Value: value}
}

// Float64 创建 float64 属性。
func Float64(key string, value float64) Attr {
	return Attr{Key: key, Value: value}
}

// Duration 创建时间间隔属性。
// OTel 转换时存储为纳秒（int64）。建议使用带单位的 key，例如 "latency_ns"。
func Duration(key string, value time.Duration) Attr {
	return Attr{Key: key, Value: value}
}

// Any 创建任意类型属性。
// 对于非标准类型（非 string/bool/int/int64/uint64/float32/float64/time.Duration），
// OTel 转换时会调用 fmt.Sprint 字符串化。推荐优先使用 [String]、[Int]、[Float32] 等类型安全函数。
func Any(key string, value any) Attr {
	return Attr{Key: key, Value: value}
}
