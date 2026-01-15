// Package xrotate 提供日志文件轮转功能
//
// # 设计理念
//
// 本包采用接口导向设计，Rotator 接口定义了轮转器的核心行为。
// 每种轮转实现有独立的构造函数和配置，保持单一职责。
//
// # 当前实现
//
//   - NewLumberjack: 基于 lumberjack v2 的按大小轮转
//
// # 文件权限说明
//
// lumberjack v2.2+ 内部默认使用 0600 权限创建日志文件（安全默认值）。
// 如需不同的权限（如 0644），可使用 WithFileMode 选项。
//
// 示例：
//
//	rotator, err := xrotate.NewLumberjack("/var/log/app.log",
//	    xrotate.WithFileMode(0644),  // 设置日志文件权限为 0644
//	)
//
// 注意：由于 lumberjack 不暴露权限配置接口，FileMode 通过写入后
// chmod 实现。存在短暂时间窗口文件权限为 0600，但对大多数场景影响可忽略。
//
// 权限会在以下场景自动应用：
//   - 首次写入创建文件后
//   - lumberjack 自动轮转创建新文件后
//   - 外部程序修改文件权限后的下一次写入
//
// # 扩展指南
//
// 如需添加新的轮转器实现，请遵循以下规范：
//
//  1. 创建新文件（如 time.go）实现 Rotator 接口
//  2. 定义独立的 Config 和 Option（如 TimeConfig, TimeOption）
//  3. 提供独立的构造函数（如 NewTimeRotator）
//  4. 不要修改 Rotator 接口，保持向后兼容
package xrotate

// Rotator 日志轮转器接口
//
// 实现 io.Writer 和 io.Closer 接口，额外提供手动轮转方法。
// 所有实现都必须是并发安全的。
//
// 扩展新实现时，必须满足以下约定：
//   - Write 必须是并发安全的
//   - Close 后不应再调用 Write（行为未定义）
//   - Rotate 可以在任意时刻调用
type Rotator interface {
	// Write 写入日志数据
	// 当触发轮转条件时自动执行轮转
	Write(p []byte) (n int, err error)

	// Close 关闭轮转器，释放资源
	Close() error

	// Rotate 手动触发日志轮转
	// 关闭当前文件，重命名为备份文件，创建新的日志文件
	Rotate() error
}
