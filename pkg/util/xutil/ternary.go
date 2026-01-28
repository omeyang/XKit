package xutil

// If 是类型安全的三目运算符。
// 使用泛型避免 interface{} 类型断言。
//
// 注意：trueVal 和 falseVal 在调用前均会求值（eager evaluation），
// 不会像 if-else 语句那样短路。如需延迟求值，请使用 if-else。
// 例如以下代码在 ptr 为 nil 时会 panic：
//
//	xutil.If(ptr != nil, ptr.Field, defaultVal) // ← 错误用法，ptr.Field 始终会求值
//
// 正确用法（仅对值类型安全）：
//
//	name := xutil.If(len(s) > 0, s, "default")
//	port := xutil.If(debug, 8080, 80)
func If[T any](cond bool, trueVal, falseVal T) T {
	if cond {
		return trueVal
	}
	return falseVal
}
