package xlog

import (
	"log/slog"
	"time"
)

// =============================================================================
// 延迟求值（Lazy Evaluation）
//
// 当日志参数计算开销较大时，使用 Lazy 系列函数可以避免在日志级别禁用时
// 执行不必要的计算。这是通过实现 slog.LogValuer 接口实现的。
//
// 工作原理：
//   1. 创建 Lazy 属性时，只保存函数引用，不执行
//   2. slog Handler 在格式化输出时调用 LogValue()
//   3. 如果日志级别被禁用，LogValue() 永远不会被调用
//
// 何时使用：
//   - 参数需要序列化大对象（JSON、XML）
//   - 参数需要数据库/网络调用
//   - 参数需要复杂计算
//   - Debug 级别日志中频繁出现的开销大参数
//
// 何时不需要：
//   - 简单的字符串、数字
//   - 已经计算好的值
//   - 生产环境必定输出的日志级别（如 Error）
// =============================================================================

// lazyValue 延迟求值的通用类型
// 实现 slog.LogValuer 接口，只有在实际输出日志时才调用 fn
type lazyValue struct {
	fn func() any
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyValue) LogValue() slog.Value {
	return slog.AnyValue(l.fn())
}

// Lazy 返回延迟求值的属性
//
// 这是最通用的延迟求值函数，适用于任意类型的值。
// Lazy 的核心价值是避免昂贵计算（fn 不被调用），而非避免分配开销。
// 由于 slog.Any 的接口装箱，即使日志级别禁用仍有 1 次堆分配（约 48B）。
// 对于简单值场景，直接使用 slog.String 等更高效。
//
// 示例 - 延迟序列化：
//
//	logger.Debug(ctx, "request",
//	    xlog.Lazy("body", func() any {
//	        return expensiveSerialize(req)
//	    }))
//
// 示例 - 延迟计算统计信息：
//
//	logger.Debug(ctx, "cache stats",
//	    xlog.Lazy("stats", func() any {
//	        return cache.GetStats() // 只在 Debug 启用时计算
//	    }))
func Lazy(key string, fn func() any) slog.Attr {
	if fn == nil {
		return slog.Any(key, nil)
	}
	return slog.Any(key, lazyValue{fn: fn})
}

// lazyStringValue 延迟求值的字符串类型
type lazyStringValue struct {
	fn func() string
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyStringValue) LogValue() slog.Value {
	return slog.StringValue(l.fn())
}

// LazyString 返回延迟求值的字符串属性
//
// 与 Lazy 类似，但专用于字符串类型，避免装箱开销。
func LazyString(key string, fn func() string) slog.Attr {
	if fn == nil {
		return slog.String(key, "")
	}
	return slog.Any(key, lazyStringValue{fn: fn})
}

// lazyIntValue 延迟求值的整数类型
type lazyIntValue struct {
	fn func() int64
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyIntValue) LogValue() slog.Value {
	return slog.Int64Value(l.fn())
}

// LazyInt 返回延迟求值的整数属性
//
// 与 Lazy 类似，但专用于 int64 类型，避免装箱开销。
func LazyInt(key string, fn func() int64) slog.Attr {
	if fn == nil {
		return slog.Int64(key, 0)
	}
	return slog.Any(key, lazyIntValue{fn: fn})
}

// lazyErrorValue 延迟求值的错误类型
type lazyErrorValue struct {
	fn func() error
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyErrorValue) LogValue() slog.Value {
	err := l.fn()
	if err == nil {
		return slog.Value{} // nil error 返回空值
	}
	return slog.StringValue(err.Error())
}

// LazyError 返回延迟求值的错误属性
//
// 用于延迟获取 error 信息，例如需要通过函数调用才能获取的错误。
// 当 fn 返回 nil 时，输出空值（JSON 中为 null）；若需完全省略字段，
// 应在日志调用前显式判断 error != nil。
func LazyError(key string, fn func() error) slog.Attr {
	if fn == nil {
		return slog.Any(key, nil)
	}
	return slog.Any(key, lazyErrorValue{fn: fn})
}

// LazyErr 返回使用标准 key "error" 的延迟错误属性
//
// 这是 LazyError(KeyError, fn) 的便捷版本。
//
// 示例：
//
//	logger.Error(ctx, "operation failed",
//	    xlog.LazyErr(func() error {
//	        return validateResult() // 只在日志输出时验证
//	    }))
func LazyErr(fn func() error) slog.Attr {
	return LazyError(KeyError, fn)
}

// lazyDurationValue 延迟求值的时间间隔类型
type lazyDurationValue struct {
	fn func() time.Duration
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyDurationValue) LogValue() slog.Value {
	return slog.StringValue(l.fn().String())
}

// LazyDuration 返回延迟求值的时间间隔属性
//
// 示例：
//
//	logger.Debug(ctx, "slow operation",
//	    xlog.LazyDuration("elapsed", func() time.Duration {
//	        return time.Since(start)
//	    }))
func LazyDuration(key string, fn func() time.Duration) slog.Attr {
	if fn == nil {
		return slog.String(key, "0s")
	}
	return slog.Any(key, lazyDurationValue{fn: fn})
}

// lazyGroupValue 延迟求值的分组类型
type lazyGroupValue struct {
	fn func() []slog.Attr
}

// LogValue 实现 slog.LogValuer 接口
func (l lazyGroupValue) LogValue() slog.Value {
	return slog.GroupValue(l.fn()...)
}

// LazyGroup 返回延迟求值的分组属性
//
// 用于延迟计算一组相关属性。
//
// 示例：
//
//	logger.Debug(ctx, "request details",
//	    xlog.LazyGroup("metrics", func() []slog.Attr {
//	        return []slog.Attr{
//	            slog.Int64("bytes", calculateBytes()),
//	            slog.Duration("latency", measureLatency()),
//	        }
//	    }))
func LazyGroup(key string, fn func() []slog.Attr) slog.Attr {
	if fn == nil {
		return slog.Group(key)
	}
	return slog.Any(key, lazyGroupValue{fn: fn})
}
