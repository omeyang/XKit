// Package xutil 提供泛型工具函数。
//
// # 功能概览
//
//   - [If]: 泛型三目运算符，编译期类型安全
//
// # 与 gobase 迁移对照
//
//	gobase                          → xutil / 说明
//	────────────────────────────────────────────────────────
//	mutils.If(cond, a, b).(T)      → xutil.If[T](cond, a, b)
//
// 其他功能已拆分至领域专属包：
//
//   - JSON 序列化 → [github.com/omeyang/xkit/pkg/util/xjson]
//   - 进程信息 → [github.com/omeyang/xkit/pkg/util/xproc]
//   - 系统资源限制 → [github.com/omeyang/xkit/pkg/util/xsys]
package xutil
