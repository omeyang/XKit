// Package xconf 提供统一的配置加载和解析功能，基于 koanf 实现。
//
// # 设计理念
//
// xconf 采用与 xcache/xmq 相同的设计模式：
//   - 工厂函数：New, NewFromBytes
//   - Client() 暴露底层 koanf 实例
//   - 增值功能：并发安全的 Reload、类型安全的 Unmarshal
//
// # 快速开始
//
//	cfg, err := xconf.New("/etc/app/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 直接使用底层 koanf 客户端
//	name := cfg.Client().String("app.name")
//
//	// 类型安全的反序列化
//	var appConfig AppConfig
//	cfg.Unmarshal("", &appConfig)
//
// # 支持的格式
//
//   - YAML（默认，推荐）：.yaml, .yml
//   - JSON：.json
//
// # 并发安全
//
// 所有方法都是并发安全的：
//   - Reload() 使用互斥锁保护
//   - Client() 返回当前 koanf 实例的指针
//   - Unmarshal() 在内部加锁
//
// 重要说明（回应常见误解）：
//
// Client() 返回的指针在 Reload() 后仍然有效，但指向旧配置。
// 这不是"并发安全问题"或"资源泄漏"，而是设计选择：
//   - 旧指针可以继续使用，不会崩溃
//   - 但数据是过期的
//
// 推荐用法：每次需要时调用 Client()，不要缓存指针：
//
//	name := cfg.Client().String("app.name")  // ✓ 推荐
//	k := cfg.Client(); name := k.String("app.name")  // ✗ 避免缓存
//
// # 配置监视
//
// 支持文件变更监视和自动重载（基于 fsnotify）：
//
//	w, _ := xconf.Watch(cfg, func(c xconf.Config, err error) {
//	    if err == nil {
//	        log.Println("config reloaded")
//	    }
//	})
//	defer w.Stop()
//	w.StartAsync()
//
// 特性：监视目录、内置防抖、并发安全、支持 vim/emacs 原子写入。
// 从 bytes 创建的 Config 不支持监视。
package xconf
