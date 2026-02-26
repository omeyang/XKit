// Package xjson 提供 JSON 序列化工具函数。
//
// 本包为 pkg/util 层级的 JSON 工具集，当前聚焦格式化输出，
// 后续可自然扩展 Compact/CompactE 等函数而不破坏现有契约。
//
// # 功能概览
//
//   - [PrettyE]: 将任意值序列化为格式化的 JSON 字符串，返回 (string, error)。
//     失败时返回空字符串和 [ErrMarshal] 包装的错误。
//   - [Pretty]: 便捷版本，用于日志和调试输出。失败时返回
//     "<marshal error: ...>" 标记字符串（非合法 JSON），便于在日志中识别序列化问题。
//
// # 注意事项
//
// 遵循 [encoding/json] 默认行为，HTML 特殊字符（<, >, &）会被转义为
// Unicode 形式（\u003c, \u003e, \u0026）。
package xjson
