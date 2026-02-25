package xenv

// Reset 重置全局状态（仅用于测试）
//
// 此函数仅在 go test 期间可用，生产代码不可调用。
// 线程安全，可在并发读取时调用。重置后 Type() 返回空字符串，IsInitialized() 返回 false。
//
// 设计决策: 先写 initialized=false 再清空 globalType，与 Init 的写入顺序（先写值再写标志）
// 对称。并发读取者在 Reset 执行期间可能短暂观察到以下中间态：
//   - initialized=true + globalType=旧有效值（Reset 尚未开始）
//   - initialized=false + globalType=旧有效值（Reset 已清标志，尚未清值）
//   - initialized=false + globalType=""（Reset 完成）
//
// 极低概率下，读取者可能先读到 initialized=true（旧值），再读到 globalType=""（新值），
// 此时 Type() 返回空字符串（符合"未初始化返回空"语义），RequireType() 通过
// dt == "" 后置校验返回 ErrNotInitialized（而非 ("", nil)），确保语义契约不被违反。
// 仅影响测试环境的 Reset 并发窗口，生产路径不受影响
// （Init/InitWith 只写一次，之后 globalType 不变）。
func Reset() {
	globalMu.Lock()
	initialized.Store(false)
	globalType.Store(DeploymentType(""))
	globalMu.Unlock()
}
