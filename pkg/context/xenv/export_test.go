package xenv

// Reset 重置全局状态（仅用于测试）
//
// 此函数仅在 go test 期间可用，生产代码不可调用。
// 线程安全，可在并发读取时调用。重置后 Type() 返回空字符串，IsInitialized() 返回 false。
//
// 设计决策: 先写 initialized=false 再清空 globalType，与 Init 的写入顺序（先写值再写标志）
// 对称。这确保并发读取者要么看到"未初始化"（initialized=false）并立即返回空值，
// 要么看到"已初始化"（initialized=true）并读到旧的有效值。
// 不会出现"已初始化 + 空值"的中间态。
func Reset() {
	globalMu.Lock()
	initialized.Store(false)
	globalType.Store(DeployType(""))
	globalMu.Unlock()
}

