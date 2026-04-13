// Package xconf 提供统一的配置加载和解析功能，基于 koanf 实现。
//
// # 设计理念
//
// xconf 定位为最小化配置加载器，负责文件/字节数据的加载、反序列化和热重载。
// 不负责配置治理（必选字段校验、默认值注入、环境变量覆盖），
// 这些能力由上层业务框架按需实现。
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
//   - Reload() 通过 sync.Mutex 序列化并发调用，防止配置回退；
//     解析成功后使用 atomic.Pointer 原子替换 koanf 实例
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
// # Unmarshal
//
// Unmarshal 使用 mapstructure 进行反序列化，默认允许弱类型转换
// （例如字符串 "8080" 可自动转为 int 8080）。
// 如需严格类型校验，建议在 Unmarshal 后自行验证（如使用 go-playground/validator）。
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
// Stop() 保证返回后不再有回调执行。在回调中调用 Stop() 是安全的，不会死锁。
package xconf
