package xenv

// Reset 重置全局状态（仅用于测试）
//
// 此函数仅在 go test 期间可用，生产代码不可调用。
// 线程安全，可在并发读取时调用。重置后 Type() 返回空字符串，IsInitialized() 返回 false。
func Reset() {
	globalMu.Lock()
	globalType.Store(DeployType(""))
	initialized.Store(false)
	globalMu.Unlock()
}
