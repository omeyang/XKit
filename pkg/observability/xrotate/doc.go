// Package xrotate 提供日志文件轮转功能。
//
// Rotator 接口定义了轮转器的核心行为（Write/Close/Rotate），所有实现并发安全。
//
// # 当前实现
//
//   - [NewLumberjack]: 基于 lumberjack v2 的按大小轮转
//
// # 生命周期
//
// Close 后调用 Write 或 Rotate 将返回 [ErrClosed]，重复调用 Close 同样返回 [ErrClosed]。
//
// # 文件权限
//
// lumberjack 默认使用 0600 权限创建日志文件。
// 如需不同权限（如 0644），使用 [WithFileMode] 选项。
// 权限调整在首次写入和轮转后执行，不在每次写入时检查。
//
// # 路径安全
//
// 设计决策: [NewLumberjack] 通过 [xfile.SanitizePath] 防止路径穿越，
// 但不限制文件必须位于特定根目录。调用方需确保 filename 来源可信。
//
// # 扩展新实现
//
//  1. 创建新文件实现 Rotator 接口
//  2. 定义独立的 Config 和 Option
//  3. 提供独立的构造函数
//  4. 不修改 Rotator 接口
package xrotate
