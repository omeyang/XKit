// Package xfile 提供通用文件系统操作工具
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
package xfile

import (
	"errors"
	"path/filepath"
	"strings"
)

func hasDotDotSegment(path string) bool {
	segments := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, seg := range segments {
		if seg == ".." {
			return true
		}
	}
	return false
}

// SanitizePath 对文件路径进行安全检查和规范化
//
// 防御措施：
//   - 路径规范化（消除 . 和 ..）
//   - 检测路径穿越攻击
//   - 拒绝空路径和纯目录路径
//
// 返回规范化后的安全路径，或错误（如果路径不安全）。
func SanitizePath(filename string) (string, error) {
	if filename == "" {
		return "", errors.New("xfile: filename is required")
	}

	// 先检查原始路径是否以分隔符结尾（表示目录）
	// 必须在 filepath.Clean 之前检查，因为 Clean 会移除尾部斜杠
	// 同时检查 / 和 \ 以确保跨平台兼容性（Windows 接受两种分隔符）
	if strings.HasSuffix(filename, "/") || strings.HasSuffix(filename, "\\") {
		return "", errors.New("xfile: invalid filename: path is a directory")
	}

	// 规范化路径
	cleaned := filepath.Clean(filename)

	// 检查路径穿越：规范化后不应包含 ".." 目录段
	//
	// 不能使用 strings.Contains(cleaned, "..")：
	//   - 会误伤合法文件名（如 "app..2024.log"）
	// 这里按路径段精确判断：只要某个 segment 恰好是 ".." 就拒绝。
	if hasDotDotSegment(cleaned) {
		return "", errors.New("xfile: invalid filename: path traversal detected")
	}

	// 获取文件名部分，确保不为空
	base := filepath.Base(cleaned)
	if base == "." || base == string(filepath.Separator) {
		return "", errors.New("xfile: invalid filename: no file name specified")
	}

	return cleaned, nil
}

// SafeJoinOptions 安全路径拼接选项
type SafeJoinOptions struct {
	// ResolveSymlinks 是否解析符号链接
	// 启用时会使用 filepath.EvalSymlinks 解析真实路径
	// 注意：要求 base 目录必须存在
	ResolveSymlinks bool
}

// SafeJoin 安全地将路径拼接到基准目录
//
// 与 SanitizePath 的区别：
//   - SanitizePath: 只检查路径格式，不验证是否在特定目录内
//   - SafeJoin: 确保结果路径始终在 base 目录内
//
// 安全保证：
//   - 拒绝绝对路径（path 必须是相对路径）
//   - 拒绝路径穿越（..）
//   - 验证最终路径以 base 为前缀
//
// 符号链接警告：
//
// 默认情况下，SafeJoin 不解析符号链接。如果 base 目录内存在指向外部的
// 符号链接，攻击者可能通过该链接访问 base 目录外的文件。
//
// 示例风险场景：
//
//	# 攻击者在 /var/log 内创建了符号链接
//	/var/log/evil -> /etc
//
//	SafeJoin("/var/log", "evil/passwd")
//	// 返回 "/var/log/evil/passwd"，但实际指向 /etc/passwd
//
// 如需防护符号链接攻击，请使用 SafeJoinWithOptions 并启用 ResolveSymlinks：
//
//	SafeJoinWithOptions(base, path, SafeJoinOptions{ResolveSymlinks: true})
//
// 使用场景：
//   - 处理用户输入的文件名
//   - 确保文件操作限制在特定目录（如日志目录、上传目录）
//
// 示例：
//
//	SafeJoin("/var/log", "app.log")       // -> "/var/log/app.log", nil
//	SafeJoin("/var/log", "../etc/passwd") // -> "", error (path traversal)
//	SafeJoin("/var/log", "/etc/passwd")   // -> "", error (absolute path)
func SafeJoin(base, path string) (string, error) {
	return SafeJoinWithOptions(base, path, SafeJoinOptions{})
}

// SafeJoinWithOptions 带选项的安全路径拼接
//
// 当 ResolveSymlinks 为 true 时：
//   - base 目录必须存在（用于解析符号链接）
//   - 会验证解析后的真实路径仍在 base 目录内
//   - 可防止通过符号链接绕过路径检查
func SafeJoinWithOptions(base, path string, opts SafeJoinOptions) (string, error) {
	// 验证并清理基础路径
	cleanBase, err := validateBase(base)
	if err != nil {
		return "", err
	}

	// 验证并清理目标路径
	cleanPath, err := validatePath(path)
	if err != nil {
		return "", err
	}

	// 拼接并验证路径
	joined, err := joinAndVerify(cleanBase, cleanPath)
	if err != nil {
		return "", err
	}

	// 如果需要解析符号链接
	if opts.ResolveSymlinks {
		return resolveAndVerifySymlinks(cleanBase, joined)
	}

	return joined, nil
}

// validateBase 验证并清理基础路径
func validateBase(base string) (string, error) {
	if base == "" {
		return "", errors.New("xfile: base directory is required")
	}
	cleanBase := filepath.Clean(base)
	if !filepath.IsAbs(cleanBase) {
		return "", errors.New("xfile: base must be an absolute path")
	}
	return cleanBase, nil
}

// validatePath 验证并清理目标路径
func validatePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("xfile: path is required")
	}
	if filepath.IsAbs(path) {
		return "", errors.New("xfile: path must be relative (absolute path not allowed)")
	}
	cleanPath := filepath.Clean(path)
	// hasDotDotSegment 已精确检测路径穿越（".." 作为独立路径段）
	// 不使用 strings.HasPrefix(cleanPath, "..") 避免误伤合法文件名如 "..config"
	if hasDotDotSegment(cleanPath) {
		return "", errors.New("xfile: path traversal detected")
	}
	return cleanPath, nil
}

// joinAndVerify 拼接路径并验证结果仍在 base 内
func joinAndVerify(cleanBase, cleanPath string) (string, error) {
	joined := filepath.Join(cleanBase, cleanPath)
	rel, err := filepath.Rel(cleanBase, joined)
	if err != nil {
		return "", errors.New("xfile: failed to compute relative path")
	}
	// 使用 hasDotDotSegment 精确检测路径穿越
	// 避免误判以 ".." 开头的合法文件名（如 "..config"）
	if hasDotDotSegment(rel) {
		return "", errors.New("xfile: path escapes base directory")
	}
	return joined, nil
}

// resolveAndVerifySymlinks 解析符号链接并验证结果路径
func resolveAndVerifySymlinks(cleanBase, joined string) (string, error) {
	realBase, err := filepath.EvalSymlinks(cleanBase)
	if err != nil {
		return "", errors.New("xfile: failed to resolve base directory symlinks")
	}

	realJoined, err := evalSymlinksPartial(joined)
	if err != nil {
		return "", errors.New("xfile: failed to resolve path symlinks")
	}

	rel, err := filepath.Rel(realBase, realJoined)
	// 使用 hasDotDotSegment 精确检测路径穿越
	if err != nil || hasDotDotSegment(rel) {
		return "", errors.New("xfile: resolved path escapes base directory")
	}

	return realJoined, nil
}

// evalSymlinksPartial 尽可能解析符号链接
// 对于不存在的文件，解析其存在的父目录部分
func evalSymlinksPartial(path string) (string, error) {
	// 先尝试直接解析
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}

	// 如果失败，尝试解析父目录
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// 递归解析父目录
		resolvedDir, err = evalSymlinksPartial(dir)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(resolvedDir, base), nil
}
