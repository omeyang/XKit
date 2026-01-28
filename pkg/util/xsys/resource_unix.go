//go:build unix

package xsys

import "golang.org/x/sys/unix"

// SetFileLimit 设置进程的最大打开文件数（RLIMIT_NOFILE）。
// 设置 soft limit 为指定值，仅在当前 hard limit 不足时提升 hard limit（需要 CAP_SYS_RESOURCE）。
// 不会降低 hard limit，因为降低 hard limit 是不可逆操作（非特权进程无法再提升）。
// 建议在进程启动时调用，多个 goroutine 并发调用可能导致 TOCTOU 竞态。
func SetFileLimit(limit uint64) error {
	if err := validateFileLimit(limit); err != nil {
		return err
	}

	var rlimit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return err
	}

	rlimit.Cur = limit
	if rlimit.Max < limit {
		rlimit.Max = limit
	}

	return unix.Setrlimit(unix.RLIMIT_NOFILE, &rlimit)
}

// GetFileLimit 查询当前进程的最大打开文件数（RLIMIT_NOFILE）。
// 返回 soft limit 和 hard limit。
func GetFileLimit() (soft, hard uint64, err error) {
	var rlimit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return 0, 0, err
	}
	return rlimit.Cur, rlimit.Max, nil
}
