package xproc

import "sync"

// ResetProcessName 重置进程名称缓存（仅用于测试）。
//
// 此函数仅在 go test 期间可用，生产代码不可调用。
func ResetProcessName() {
	processNameOnce = sync.Once{}
	processNameValue = ""
}
