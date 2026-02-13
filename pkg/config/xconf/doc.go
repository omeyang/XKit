// Package xconf 提供统一的配置加载和解析功能，基于 koanf 实现。
//
// # 设计理念
//
// xconf 采用与 xcache/xmq 相同的设计模式：
//   - 工厂函数：New, NewFromBytes
//   - Client() 暴露底层 koanf 实例
//   - 增值功能：并发安全的 Reload、类型安全的 Unmarshal
//
// # 支持的格式
//
//   - YAML（默认，推荐）：.yaml, .yml
//   - JSON：.json
//
// # 并发安全
//
// 所有方法都是并发安全的：
//   - Reload() 使用 atomic.Pointer 原子替换 koanf 实例
//   - Client() 通过 atomic.Load 返回当前 koanf 实例指针（无锁）
//   - Unmarshal() 通过 atomic.Load 获取实例后在 koanf 内部锁保护下反序列化
//
// Client() 返回的指针在 Reload() 后仍然有效，但指向旧配置。
// 这是设计选择（快照语义），不是并发安全问题：
//   - 旧指针可以继续使用，不会崩溃
//   - 但数据是过期的
//
// 推荐用法：每次需要时调用 Client()，不要长期缓存返回的指针。
//
// # MustUnmarshal
//
// MustUnmarshal 是包级函数（非接口方法），适用于程序启动时的必要配置加载：
//
//	xconf.MustUnmarshal(cfg, "database", &dbConfig) // 失败时 panic
//
// # 配置监视
//
// 支持文件变更监视和自动重载（基于 fsnotify）。
// 特性：监视目录、内置防抖、并发安全、支持 vim/emacs 原子写入。
// 从 bytes 创建的 Config 不支持监视。
// Stop() 保证返回后不再有回调执行。
package xconf
