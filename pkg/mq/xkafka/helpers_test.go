package xkafka

// errorString 安全地获取错误字符串，nil 返回空字符串。
// 仅在测试中使用，生产代码通过 FailureReasonFormatter 处理错误格式化。
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
