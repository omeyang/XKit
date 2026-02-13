//go:build unix

package xsys

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// 系统调用函数变量，支持测试中 mock 替换以覆盖错误路径。
// 设计决策: 使用包级变量 mock 模式（与 xproc.osExecutable 一致），对此规模的包足够简洁。
// 注意：mock 测试不可使用 t.Parallel()，因为替换包级变量会引发竞态。
var (
	getrlimit = unix.Getrlimit
	setrlimit = unix.Setrlimit
)

// fileLimitMu 保护 SetFileLimit 的 getrlimit→setrlimit 读改写序列，
// 避免进程内并发调用导致互相覆盖。
var fileLimitMu sync.Mutex

// SetFileLimit 设置进程的最大打开文件数（RLIMIT_NOFILE）。
// 设置 soft limit 为指定值，仅在当前 hard limit 不足时提升 hard limit（需要 CAP_SYS_RESOURCE）。
// 不会降低 hard limit，因为降低 hard limit 是不可逆操作（非特权进程无法再提升）。
// 并发安全：内部使用互斥锁保护读改写序列。
func SetFileLimit(limit uint64) error {
	if err := validateFileLimit(limit); err != nil {
		return err
	}

	fileLimitMu.Lock()
	defer fileLimitMu.Unlock()

	var rlimit unix.Rlimit
	if err := getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("xsys: getrlimit RLIMIT_NOFILE: %w", err)
	}

	rlimit.Cur = limit
	if rlimit.Max < limit {
		rlimit.Max = limit
	}

	if err := setrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("xsys: setrlimit RLIMIT_NOFILE: %w", err)
	}
	return nil
}

// GetFileLimit 查询当前进程的最大打开文件数（RLIMIT_NOFILE）。
// 返回 soft limit 和 hard limit。
// 并发安全：单次系统调用，无需互斥保护。
func GetFileLimit() (soft, hard uint64, err error) {
	var rlimit unix.Rlimit
	if err := getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return 0, 0, fmt.Errorf("xsys: getrlimit RLIMIT_NOFILE: %w", err)
	}
	return rlimit.Cur, rlimit.Max, nil
}
