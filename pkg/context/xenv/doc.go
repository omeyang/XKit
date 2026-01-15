// Package xenv 提供部署环境信息的管理。
//
// # 核心理念
//
// xenv 管理环境级别的配置信息，这些信息：
//   - 在服务启动时确定
//   - 整个生命周期内不变
//   - 所有请求共享相同的值
//
// # 支持的部署类型
//
//   - LOCAL: 本地/私有化部署
//   - SAAS: SaaS 云部署
//
// # 快速开始
//
// 服务启动时初始化：
//
//	func main() {
//	    // 从环境变量 DEPLOYMENT_TYPE 初始化
//	    if err := xenv.Init(); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // 或者失败时 panic
//	    xenv.MustInit()
//
//	    // 现在可以使用全局函数
//	    fmt.Println("Deploy Type:", xenv.Type())
//	    fmt.Println("Is Local:", xenv.IsLocal())
//	    fmt.Println("Is SaaS:", xenv.IsSaaS())
//	}
//
// # 环境变量
//
// xenv 从以下环境变量读取配置：
//
//   - DEPLOYMENT_TYPE: 部署类型，值为 "LOCAL" 或 "SAAS"（大小写不敏感）
//
// # 线程安全
//
// 所有导出函数都是线程安全的：
//
//   - Init/MustInit 只应在 main() 中调用一次
//   - Type/IsLocal/IsSaaS 可并发调用
package xenv
