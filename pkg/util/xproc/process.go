package xproc

import (
	"os"
	"path/filepath"
	"sync"
)

// osExecutable 是 os.Executable 的包级变量，支持测试中 mock。
//
// 设计决策: 使用包级变量 mock 是 Go 生态中广泛使用的测试模式，
// 对于包规模极小（仅 2 个导出函数）的场景，此方案的简洁性优于依赖注入。
var osExecutable = os.Executable

// processName 缓存进程名称，避免每次调用都执行 readlink 系统调用。
var (
	processNameOnce  sync.Once
	processNameValue string
)

// ProcessID 返回当前进程 ID。
func ProcessID() int {
	return os.Getpid()
}

// baseName 提取路径的基础文件名。
// 对 [filepath.Base] 返回的特殊值（"."、".."、路径分隔符）返回空字符串。
func baseName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

// resolveProcessName 执行实际的进程名称解析。
func resolveProcessName() string {
	if exe, err := osExecutable(); err == nil && exe != "" {
		if name := baseName(exe); name != "" {
			return name
		}
	}
	if len(os.Args) == 0 || os.Args[0] == "" {
		return ""
	}
	return baseName(os.Args[0])
}

// ProcessName 返回当前进程名称（不含路径）。
// 结果在首次调用时缓存（包括空字符串），后续调用直接返回缓存值，无系统调用开销。
//
// 优先使用 [os.Executable] 获取可执行文件路径（不受 os.Args 修改影响），
// 失败时回退到 os.Args[0]。
// 对 [filepath.Base] 返回的特殊值（"."、".."、路径分隔符）统一返回空字符串。
// 在极端情况下（所有来源均无效）返回空字符串，调用方可据此判断是否获取成功。
//
// 设计决策: 返回 string 而非 (string, error)。ProcessName 的典型用途是日志字段、
// 指标标签等"尽力获取"场景，调用方通常只需要一个字符串，不需要区分失败原因。
// 强制返回 error 会导致每个调用点都需要 if err != nil { name = "unknown" }，
// 降低了 API 的便利性，且空字符串本身已是充分的"失败"信号。
//
// 设计决策: 失败结果（空字符串）也会被永久缓存，不会重试。在标准 Go 进程中，
// [os.Executable] 和 os.Args[0] 同时不可用的概率极低（Linux 通过 /proc/self/exe 获取，
// 运行时保证 os.Args 已初始化）。引入重试逻辑会增加 sync.Once 之外的同步复杂度，
// 对于进程标识这一启动即确定的信息，收益不足以抵消复杂度成本。
func ProcessName() string {
	processNameOnce.Do(func() {
		processNameValue = resolveProcessName()
	})
	return processNameValue
}
