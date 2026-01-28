// Package xproc 提供当前进程信息查询工具。
//
// # 功能概览
//
//   - [ProcessID]: 获取当前进程 PID
//   - [ProcessName]: 获取当前进程名称（不含路径）
//
// # 与 gobase 迁移对照
//
//	gobase                          → xproc / 说明
//	────────────────────────────────────────────────────────
//	msys.ProcessId()                → xproc.ProcessID()
//	msys.ProcessName()              → xproc.ProcessName()
package xproc
