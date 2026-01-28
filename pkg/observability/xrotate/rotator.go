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
