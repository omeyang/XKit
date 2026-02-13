package xutil

// If 是类型安全的三目运算符。
// 使用泛型避免 interface{} 类型断言。
//
// 设计决策: 命名为 If 而非 Ternary/Cond，与 Go 社区惯例一致（如 samber/lo.If）。
// 泛型约束使用 any 而非 comparable，因为函数仅做值传递，不需要比较能力。
//
// 与标准库 [cmp.Or] 的区别：cmp.Or 返回首个非零值（零值判定），
// If 根据布尔条件选择分支（条件判定），两者语义不同，不可互替。
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
