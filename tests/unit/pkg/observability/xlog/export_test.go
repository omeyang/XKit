package xlog

// SetNewBuilderForTest 替换 defaultLogger 使用的构建器工厂（仅用于测试 fallback 路径）。
// 返回恢复函数，测试结束时必须调用。
func SetNewBuilderForTest(fn func() *Builder) func() {
	old := newBuilder
	newBuilder = fn
	return func() { newBuilder = old }
}
