// Package xjson 提供 JSON 序列化工具函数。
//
// # 功能概览
//
//   - [PrettyE]: 将任意值序列化为格式化的 JSON 字符串，返回 (string, error)。
//     失败时返回空字符串和 [ErrMarshal] 包装的错误。
//   - [Pretty]: 便捷版本，用于日志和调试输出。失败时返回
//     "<marshal error: ...>" 标记字符串（非合法 JSON），便于在日志中识别序列化问题。
package xjson
