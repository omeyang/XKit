// Package xsys 提供系统资源限制管理工具。
//
// # 功能概览
//
//   - [SetFileLimit]: 设置进程最大打开文件数（Unix 平台生效，非 Unix 返回 [ErrUnsupportedPlatform]）
//   - [GetFileLimit]: 查询进程当前最大打开文件数（Unix 平台生效，非 Unix 返回 [ErrUnsupportedPlatform]）
//
// # 平台支持
//
// SetFileLimit 和 GetFileLimit 在所有 Unix 平台（Linux、macOS、FreeBSD 等）上
// 通过 RLIMIT_NOFILE 系统调用实现。在 Windows 等非 Unix 平台上返回 [ErrUnsupportedPlatform]。
// 参数校验（如零值检查）在所有平台上行为一致。
//
// # 与 gobase 迁移对照
//
//	gobase                          → xsys / 说明
//	────────────────────────────────────────────────────────
//	resource.SetLimits(cur)         → xsys.SetFileLimit(limit)
//	resource.SetContainerAwareness()→ 已移除：Go 1.24+ 运行时原生支持 cgroup CPU 限制感知
package xsys
