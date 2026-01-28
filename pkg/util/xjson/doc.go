// Package xjson 提供 JSON 序列化工具函数。
//
// # 功能概览
//
//   - [Pretty]: 将任意值序列化为格式化的 JSON 字符串，用于日志和调试输出
//
// # 与 gobase 迁移对照
//
//	gobase                          → xjson / 说明
//	────────────────────────────────────────────────────────
//	mutils.Struct2Str(v)            → xjson.Pretty(v)
package xjson
