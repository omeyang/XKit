package xfile

import (
	"fmt"
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
// 功能：
//   - 路径规范化（消除 . 和冗余斜杠）
//   - 阻止相对路径穿越（如 "../etc/passwd"）
//   - 拒绝空路径和纯目录路径
//
// 重要限制：
//   - 绝对路径的 ".." 会被 filepath.Clean 正常解析
//   - 例如："/var/log/../etc" -> "/etc"（这是合法的绝对路径，不是穿越）
//   - 如需将路径限制在特定目录内，请使用 SafeJoin
//
// 适用场景：
//   - 验证用户输入的文件名格式
//   - 防止相对路径穿越攻击
//
// 不适用场景：
//   - 需要将文件限制在特定目录内（请用 SafeJoin）
//   - 需要防止绝对路径访问敏感文件（请用系统权限控制）
//
// 返回规范化后的路径，或错误（如果路径格式无效）。
func SanitizePath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename is required: %w", ErrEmptyPath)
	}

	// 先检查原始路径是否以分隔符结尾（表示目录）
	// 必须在 filepath.Clean 之前检查，因为 Clean 会移除尾部斜杠
	// 同时检查 / 和 \ 以确保跨平台兼容性（Windows 接受两种分隔符）
	if strings.HasSuffix(filename, "/") || strings.HasSuffix(filename, "\\") {
		return "", fmt.Errorf("path is a directory: %w", ErrInvalidPath)
	}

	// 规范化路径
	cleaned := filepath.Clean(filename)

	// 检查路径穿越：规范化后不应包含 ".." 目录段
	//
	// 不能使用 strings.Contains(cleaned, "..")：
	//   - 会误伤合法文件名（如 "app..2024.log"）
	// 这里按路径段精确判断：只要某个 segment 恰好是 ".." 就拒绝。
	if hasDotDotSegment(cleaned) {
		return "", fmt.Errorf("path traversal in filename: %w", ErrPathTraversal)
	}

	// 获取文件名部分，确保不为空
	base := filepath.Base(cleaned)
	if base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf("no file name specified: %w", ErrInvalidPath)
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
//   - 降低通过符号链接绕过路径检查的风险
//
// 注意：符号链接检查存在 TOCTOU（Time-of-Check-Time-of-Use）窗口，
// 即检查与实际使用之间符号链接可能被修改。对于需要强安全保证的场景，
// 建议配合操作系统级别的目录权限控制（如禁止在 base 目录内创建符号链接）。
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
		return "", fmt.Errorf("base directory is required: %w", ErrEmptyPath)
	}
	cleanBase := filepath.Clean(base)
	if !filepath.IsAbs(cleanBase) {
		return "", fmt.Errorf("base must be an absolute path: %w", ErrInvalidPath)
	}
	return cleanBase, nil
}

// validatePath 验证并清理目标路径
func validatePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required: %w", ErrEmptyPath)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative (absolute path not allowed): %w", ErrInvalidPath)
	}
	cleanPath := filepath.Clean(path)
	// hasDotDotSegment 已精确检测路径穿越（".." 作为独立路径段）
	// 不使用 strings.HasPrefix(cleanPath, "..") 避免误伤合法文件名如 "..config"
	if hasDotDotSegment(cleanPath) {
		return "", ErrPathTraversal
	}
	return cleanPath, nil
}

// joinAndVerify 拼接路径并验证结果仍在 base 内
func joinAndVerify(cleanBase, cleanPath string) (string, error) {
	joined := filepath.Join(cleanBase, cleanPath)
	rel, err := filepath.Rel(cleanBase, joined)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path (%v): %w", err, ErrPathEscaped)
	}
	// 使用 hasDotDotSegment 精确检测路径穿越
	// 避免误判以 ".." 开头的合法文件名（如 "..config"）
	if hasDotDotSegment(rel) {
		return "", ErrPathEscaped
	}
	return joined, nil
}

// resolveAndVerifySymlinks 解析符号链接并验证结果路径
func resolveAndVerifySymlinks(cleanBase, joined string) (string, error) {
	realBase, err := filepath.EvalSymlinks(cleanBase)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory symlinks: %w", ErrSymlinkResolution)
	}

	realJoined, err := evalSymlinksPartial(joined)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path symlinks: %w", ErrSymlinkResolution)
	}

	rel, err := filepath.Rel(realBase, realJoined)
	// 使用 hasDotDotSegment 精确检测路径穿越
	if err != nil || hasDotDotSegment(rel) {
		return "", fmt.Errorf("resolved path escapes base directory: %w", ErrPathEscaped)
	}

	return realJoined, nil
}

// maxSymlinkDepth 是 evalSymlinksPartial 递归的最大深度
// 与 Linux 路径最大深度一致，防止栈溢出
const maxSymlinkDepth = 255

// evalSymlinksPartial 尽可能解析符号链接
// 对于不存在的文件，解析其存在的父目录部分
func evalSymlinksPartial(path string) (string, error) {
	return evalSymlinksPartialWithDepth(path, 0)
}

// evalSymlinksPartialWithDepth 带深度限制的符号链接解析
func evalSymlinksPartialWithDepth(path string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", ErrPathTooDeep
	}

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
		resolvedDir, err = evalSymlinksPartialWithDepth(dir, depth+1)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(resolvedDir, base), nil
}
