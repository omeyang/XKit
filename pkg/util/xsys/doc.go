// Package xsys 提供系统资源限制管理工具。
//
// # 功能概览
//
//   - [SetFileLimit]: 设置进程最大打开文件数（Unix 平台生效）。
//     非法参数优先返回 [ErrInvalidFileLimit]；合法参数在非 Unix 平台返回 [ErrUnsupportedPlatform]。
//     当请求值超过当前 hard limit 时需要 CAP_SYS_RESOURCE（或 root 权限），否则返回 EPERM。
//     上界由操作系统内核参数（如 Linux fs.nr_open）决定，超出时返回 EPERM 或 EINVAL。
//   - [GetFileLimit]: 查询进程当前最大打开文件数（Unix 平台生效，非 Unix 返回 [ErrUnsupportedPlatform]）
//
// # 错误哨兵
//
//   - [ErrInvalidFileLimit]: 文件限制值无效（如零值）
//   - [ErrUnsupportedPlatform]: 当前平台不支持此操作
//
// # 平台支持
//
// SetFileLimit 和 GetFileLimit 在所有 Unix 平台（Linux、macOS、FreeBSD 等）上
// 通过 RLIMIT_NOFILE 系统调用实现。在 Windows 等非 Unix 平台上返回 [ErrUnsupportedPlatform]。
// 参数校验（如零值检查）在所有平台上行为一致。
package xsys
