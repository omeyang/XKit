// Package xfile 提供通用文件系统操作工具。
//
// 本包提供安全、便捷的文件和目录操作函数，是逐步积累的工具集。
// 所有函数都考虑了安全性（如路径穿越防护）和跨平台兼容性。
//
// # 路径安全函数对比
//
//   - SanitizePath: 检查路径格式，防止相对路径穿越，不限制目标目录
//   - SafeJoin: 确保结果路径始终在指定的 base 目录内，推荐用于处理用户输入
//   - SafeJoinWithOptions: SafeJoin 的增强版，支持符号链接解析等选项
//
// # 路径穿越检测
//
// 路径穿越检测使用精确的路径段匹配，只有 ".." 作为独立路径段时才被视为穿越攻击。
// 以 ".." 开头的合法文件名（如 "..config"、"...hidden"）不会被误判：
//
//	SafeJoin("/var/log", "..config")      // ✓ 合法 -> "/var/log/..config"
//	SafeJoin("/var/log", "../etc/passwd") // ✗ 拒绝 -> 路径穿越
//
// # 空字节防护
//
// SanitizePath 和 SafeJoin 均拒绝包含空字节（\x00）的路径。Linux 内核在 VFS 层
// 会在空字节处截断路径，导致 Go 代码与操作系统实际操作的路径不一致。
//
// # URL 编码注意事项（前置条件）
//
// 本包处理文件系统路径，不处理 URL 编码。"%2e%2e"、"%2f"、"%5c" 等编码序列
// 被视为合法的文件名字符，不会被识别为路径穿越或分隔符。
//
// 重要：如果输入来自 HTTP 请求，调用方必须先完成 URL 解码再传入本包的任何函数。
// 未解码的 URL 路径可能绕过穿越检测。
//
// # 符号链接安全注意事项
//
// SafeJoin 默认不解析符号链接，这在大多数场景下是合适的（如日志目录）。
// 但如果 base 目录内可能存在恶意符号链接，攻击者可能通过符号链接访问
// base 目录外的文件。
//
// 对于高安全场景（如用户上传、沙箱目录），应使用 SafeJoinWithOptions
// 并启用 ResolveSymlinks 选项：
//
//	SafeJoinWithOptions(base, path, SafeJoinOptions{ResolveSymlinks: true})
//
// 重要限制：SafeJoin 系列函数返回"经过验证的路径字符串"，检查与实际文件操作
// 之间存在 TOCTOU 窗口。本包适用于可信环境下的路径构建，不能替代对抗性环境下
// 的安全文件访问。如需更强保证，应配合操作系统权限控制（如禁止在 base 内创建
// 符号链接）或使用专门的安全文件访问库。
//
// # 错误处理
//
// 预定义错误变量支持 [errors.Is] 判断：
//
//	_, err := xfile.SafeJoin("/var/log", "../etc/passwd")
//	if errors.Is(err, xfile.ErrPathTraversal) {
//	    // 处理路径穿越
//	}
package xfile
