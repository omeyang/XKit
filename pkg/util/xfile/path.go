package xfile

import (
	"fmt"
	"path/filepath"
	"strings"
)

// containsNullByte 检测路径是否包含空字节。
// Linux 内核在 VFS 层会在空字节处截断路径，导致 Go 代码与操作系统看到的路径不一致。
func containsNullByte(path string) bool {
	return strings.ContainsRune(path, 0)
}

// isWindowsAbsPath 检测 Windows 风格的绝对或驱动器相关路径。
// 在非 Windows 平台上，filepath.IsAbs 不识别 "C:\..." 或 "\\server\..." 形式，
// 需要显式检测以防止跨平台场景下的安全策略绕过。
//
// 检测的 Windows 路径形式：
//   - 驱动器绝对路径: "C:\..." 或 "C:/..."
//   - 驱动器相对路径: "C:foo"（当前驱动器工作目录下的相对路径）
//   - UNC 路径: "\\server\..."
//   - 根路径: "\Windows\..." (当前驱动器的根目录)
func isWindowsAbsPath(path string) bool {
	// 驱动器号: "C:\..."、"C:/..." 以及驱动器相对路径 "C:foo"
	// 一律拒绝任何 "X:" 开头的路径，避免 Windows 驱动器相关的语义歧义
	if len(path) >= 2 && isASCIILetter(path[0]) && path[1] == ':' {
		return true
	}
	// Windows 根路径 "\foo\..." 或 UNC 路径 "\\server\..."
	// 任何以反斜杠开头的路径在 Windows 上都是绝对路径（根路径或 UNC），
	// 在 Linux 上反斜杠开头的文件名极为罕见，为安全起见一并拒绝。
	return len(path) >= 1 && path[0] == '\\'
}

func isASCIILetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// hasDotDotSegment 检测路径中是否包含 ".." 作为独立路径段。
// 使用逐字符扫描实现零内存分配，避免 strings.FieldsFunc 的 []string 开销。
// 同时将 '/' 和 '\' 视为分隔符，以检测 Windows 风格路径穿越（即使在 Linux 上）。
func hasDotDotSegment(path string) bool {
	i := 0
	for i < len(path) {
		// 跳过分隔符
		if path[i] == '/' || path[i] == '\\' {
			i++
			continue
		}
		// 找到段的结束位置
		j := i
		for j < len(path) && path[j] != '/' && path[j] != '\\' {
			j++
		}
		// 检查段是否恰好为 ".."
		if j-i == 2 && path[i] == '.' && path[i+1] == '.' {
			return true
		}
		i = j
	}
	return false
}

// SanitizePath 对文件路径进行安全检查和规范化
//
// 安全边界：本函数仅做格式净化，不防护绝对路径穿越。
// 如需将路径限制在特定目录内，请使用 [SafeJoin] 或 [SafeJoinWithOptions]。
//
// 功能：
//   - 路径规范化（消除 . 和冗余斜杠）
//   - 阻止相对路径穿越（如 "../etc/passwd"）
//   - 拒绝空路径和显式目录路径（尾随 "/" 或 "\"）
//
// 重要限制：
//   - 本函数接受绝对路径（包括 Windows 格式如 "C:\..." 和 UNC "\\server\..."）
//   - 绝对路径的 ".." 会被 filepath.Clean 正常解析
//   - 例如："/var/log/../etc" -> "/etc"（这是合法的绝对路径，不是穿越）
//   - 如需将路径限制在特定目录内或拒绝绝对路径，请使用 SafeJoin
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
//
// 设计决策: 函数名 SanitizePath 表示"格式净化"（空路径、空字节、穿越、目录路径），
// 不等同于"沙箱隔离"。如需目录隔离语义，请使用 SafeJoin。
func SanitizePath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename is required: %w", ErrEmptyPath)
	}

	if containsNullByte(filename) {
		return "", fmt.Errorf("filename contains null byte: %w", ErrNullByte)
	}

	// 先检查原始路径是否以分隔符结尾（表示目录）
	// 必须在 filepath.Clean 之前检查，因为 Clean 会移除尾部斜杠
	// 同时检查 / 和 \ 以确保跨平台兼容性：Windows 接受两种分隔符，
	// 拒绝尾部 \ 可防止 Windows 路径被误传入后产生语义歧义
	//
	// 设计决策: 在 Linux 上反斜杠是合法的文件名字符，以 "\" 结尾的文件名理论上合法，
	// 但极为罕见且几乎总是跨平台拼接错误。为安全起见统一拒绝，避免语义歧义。
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
//
// 设计决策: 默认不解析符号链接，因为启用后要求 base 目录在文件系统上存在，
// 会破坏纯路径构建场景（如配置阶段目录尚未创建）。高安全场景应用 SafeJoinWithOptions。
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
// 安全边界（重要）：
//
// 本函数返回"经过验证的路径字符串"，而非直接打开文件。检查与实际 open/write
// 之间存在 TOCTOU（Time-of-Check-Time-of-Use）窗口，攻击者可能在此期间替换
// 符号链接或目录结构。本函数适用于可信环境下的路径构建（如日志目录、配置路径），
// 不能替代对抗性环境下的安全文件访问。
//
// 设计决策: 不提供基于 openat/O_NOFOLLOW 的原子化文件操作，因为这会将 xfile
// 从"路径工具库"变为"安全文件访问库"，超出本包的职责范围。对抗性场景应结合
// 操作系统级别的目录权限控制（如禁止在 base 目录内创建符号链接）或使用专门的
// 安全文件访问库。
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
	if containsNullByte(base) {
		return "", fmt.Errorf("base contains null byte: %w", ErrNullByte)
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
	if containsNullByte(path) {
		return "", fmt.Errorf("path contains null byte: %w", ErrNullByte)
	}
	if filepath.IsAbs(path) || isWindowsAbsPath(path) {
		return "", fmt.Errorf("path must be relative (absolute path not allowed): %w", ErrInvalidPath)
	}
	cleanPath := filepath.Clean(path)
	// hasDotDotSegment 已精确检测路径穿越（".." 作为独立路径段）
	// 不使用 strings.HasPrefix(cleanPath, "..") 避免误伤合法文件名如 "..config"
	if hasDotDotSegment(cleanPath) {
		return "", fmt.Errorf("path traversal in path: %w", ErrPathTraversal)
	}
	return cleanPath, nil
}

// filepathRelFn 用于 joinAndVerify 中的路径关系计算。
// 默认为 filepath.Rel，测试中可注入模拟实现以覆盖防御性错误分支。
// 测试注入点：非并发安全，同包测试中修改此变量的用例不应使用 t.Parallel()。
var filepathRelFn = filepath.Rel

// joinAndVerify 拼接路径并验证结果仍在 base 内
//
// 设计决策: filepath.Rel 对两个已清理的绝对路径不会返回错误，此处的错误分支
// 是防御性代码，防止标准库行为变更时出现静默安全漏洞。
func joinAndVerify(cleanBase, cleanPath string) (string, error) {
	joined := filepath.Join(cleanBase, cleanPath)
	rel, err := filepathRelFn(cleanBase, joined)
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
		return "", fmt.Errorf("resolve base directory symlinks: %w: %w", ErrSymlinkResolution, err)
	}

	realJoined, err := evalSymlinksPartial(joined)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w: %w", ErrSymlinkResolution, err)
	}

	rel, err := filepath.Rel(realBase, realJoined)
	// 使用 hasDotDotSegment 精确检测路径穿越
	if err != nil || hasDotDotSegment(rel) {
		return "", fmt.Errorf("resolved path escapes base directory: %w", ErrPathEscaped)
	}

	return realJoined, nil
}

// maxSymlinkDepth 是 evalSymlinksPartial 向上查找可解析祖先时的最大层数限制。
const maxSymlinkDepth = 255

// evalSymlinksPartial 尽可能解析符号链接
// 对于不存在的文件，解析其存在的父目录部分。
//
// 符号链接循环行为：当路径中存在符号链接循环时，filepath.EvalSymlinks 对包含循环的
// 路径段会返回 ELOOP 错误。本函数会跳过该层继续向上查找可解析祖先，最终返回的路径
// 可能仍包含未解析的循环符号链接段。此行为在实际中风险极低：(1) 符号链接循环在生产
// 环境中罕见；(2) 打开文件时 OS 会返回 ELOOP 错误；(3) 调用方（resolveAndVerifySymlinks）
// 仍会对返回路径执行包含性检查。
func evalSymlinksPartial(path string) (string, error) {
	// 快速路径：直接解析
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}

	// 迭代：从叶向根逐层收集不存在的路径段，找到最深的可解析祖先
	clean := filepath.Clean(path)
	var trail []string // 不存在的路径段（逆序收集）

	current := clean
	for i := 0; ; i++ {
		if i > maxSymlinkDepth {
			return "", ErrPathTooDeep
		}

		dir := filepath.Dir(current)
		base := filepath.Base(current)

		if dir == current {
			// 设计决策: 已到根目录但 EvalSymlinks 仍失败（理论上不应发生，
			// 因为 "/" 总是可解析的），视为路径过深以确保循环终止。
			return "", ErrPathTooDeep
		}

		trail = append(trail, base)

		resolved, err = filepath.EvalSymlinks(dir)
		if err == nil {
			// 找到可解析的祖先，逆序追加不存在的段
			for j := len(trail) - 1; j >= 0; j-- {
				resolved = filepath.Join(resolved, trail[j])
			}
			return resolved, nil
		}

		current = dir
	}
}
