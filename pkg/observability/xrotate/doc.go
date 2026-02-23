// Package xrotate 提供日志文件轮转功能。
//
// Rotator 接口定义了轮转器的核心行为（Write/Close/Rotate），所有实现并发安全。
//
// # 当前实现
//
//   - [NewLumberjack]: 基于 lumberjack v2 的按大小轮转
//
// 注意：当前仅支持基于文件大小的轮转策略，不支持基于时间（如每日轮转）的策略。
// 如需基于时间的轮转，建议配合外部工具（如 logrotate）使用。
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
// # 错误处理
//
// 内部操作（如文件权限调整）的错误通过 [WithOnError] 回调上报。
// 回调 panic 被 recover 隔离，不会传播到业务调用链。
// 设计决策: 不在底层 writer 内使用 slog 等日志库记录错误，
// 避免 Rotator 作为日志输出目标时产生递归写入导致死锁或栈溢出。
//
// # 路径安全
//
// 设计决策: [NewLumberjack] 通过 [xfile.SanitizePath] 防止路径穿越，
// 但不限制文件必须位于特定根目录。调用方需确保 filename 来源可信。
//
// # 已知限制
//
// 设计决策: lumberjack v2 的 Close 不关闭内部 millCh channel，导致
// 负责文件压缩和清理的 millRun goroutine 在 Logger 关闭后仍驻留。
// 该 goroutine 随进程退出回收，不影响正常运行；但在长期运行且多次
// 创建/关闭 Rotator 的场景中会线性累积。这是上游已知限制，
// 无法在 wrapper 层修复。测试通过 goleak 白名单过滤。
//
// 使用建议: 应复用 Rotator 实例，避免在运行时反复创建和销毁（如动态重载
// 日志配置时替换 Rotator）。如需更换日志文件，优先使用 Rotate 方法。
//
// # 扩展新实现
//
//  1. 创建新文件实现 Rotator 接口
//  2. 定义独立的 Config 和 Option
//  3. 提供独立的构造函数
//  4. 不修改 Rotator 接口
package xrotate
