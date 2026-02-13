package xrotate

import "io"

// 编译时断言：Rotator 接口是 io.WriteCloser 的超集
var _ io.WriteCloser = (Rotator)(nil)

// Rotator 日志轮转器接口
//
// 隐式实现 [io.WriteCloser]，可直接用于任何接受 io.Writer 或
// io.WriteCloser 的场景（如 xlog 的输出目标）。
// 额外提供 Rotate 方法用于手动触发轮转。
// 所有实现都必须是并发安全的。
//
// 扩展新实现时，必须满足以下约定：
//   - Write 必须是并发安全的
//   - Close 后调用 Write 或 Rotate 应返回 [ErrClosed]
//   - Rotate 可以在任意时刻调用
type Rotator interface {
	// Write 写入日志数据
	// 当触发轮转条件时自动执行轮转
	Write(p []byte) (n int, err error)

	// Close 关闭轮转器，释放资源
	// 重复调用应返回 [ErrClosed]
	Close() error

	// Rotate 手动触发日志轮转
	// 关闭当前文件，重命名为备份文件，创建新的日志文件
	Rotate() error
}
