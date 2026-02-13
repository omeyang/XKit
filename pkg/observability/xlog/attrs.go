package xlog

import (
	"log/slog"
	"time"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// 常用属性 Key 常量
//
// 定义日志中常用的标准字段名，保持一致性。
// 参考 OpenTelemetry Semantic Conventions。
// =============================================================================

const (
	// KeyError 错误字段的标准 key
	KeyError = "error"

	// KeyStack 堆栈字段的标准 key
	KeyStack = "stack"

	// KeyDuration 耗时字段的标准 key
	KeyDuration = "duration"

	// KeyCount 计数字段的标准 key
	KeyCount = "count"

	// KeyUserID 用户 ID 字段的标准 key
	KeyUserID = "user_id"

	// KeyRequestID 请求 ID 字段的标准 key，引用 xctx 保证跨包一致
	KeyRequestID = xctx.KeyRequestID

	// KeyMethod HTTP/RPC 方法字段的标准 key
	KeyMethod = "method"

	// KeyPath 请求路径字段的标准 key
	KeyPath = "path"

	// KeyStatusCode HTTP 状态码字段的标准 key
	KeyStatusCode = "status_code"

	// KeyComponent 组件名称字段的标准 key
	KeyComponent = "component"

	// KeyOperation 操作名称字段的标准 key
	KeyOperation = "operation"
)

// =============================================================================
// 便捷属性构造函数
// =============================================================================

// Err 创建错误属性
//
// 这是记录错误的标准方式，使用统一的 key "error"。
// 如果 err 为 nil，返回空属性（会被忽略）。
//
// 示例：
//
//	if err != nil {
//	    logger.Error(ctx, "operation failed", xlog.Err(err))
//	}
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{} // 空属性会被 slog 忽略
	}
	return slog.String(KeyError, err.Error())
}

// Duration 创建耗时属性
//
// 示例：
//
//	start := time.Now()
//	// ... 操作 ...
//	logger.Info(ctx, "operation completed", xlog.Duration(time.Since(start)))
func Duration(d time.Duration) slog.Attr {
	return slog.String(KeyDuration, d.String())
}

// Component 创建组件名属性
//
// 用于标识日志来源组件。
func Component(name string) slog.Attr {
	return slog.String(KeyComponent, name)
}

// Operation 创建操作名属性
//
// 用于标识当前执行的操作。
func Operation(name string) slog.Attr {
	return slog.String(KeyOperation, name)
}

// Count 创建计数属性
func Count(n int64) slog.Attr {
	return slog.Int64(KeyCount, n)
}

// UserID 创建用户 ID 属性
func UserID(id string) slog.Attr {
	return slog.String(KeyUserID, id)
}

// StatusCode 创建 HTTP 状态码属性
func StatusCode(code int) slog.Attr {
	return slog.Int(KeyStatusCode, code)
}

// Method 创建 HTTP/RPC 方法属性
func Method(m string) slog.Attr {
	return slog.String(KeyMethod, m)
}

// Path 创建请求路径属性
func Path(p string) slog.Attr {
	return slog.String(KeyPath, p)
}
