// Package xenv 提供部署环境信息的管理。
//
// # 核心理念
//
// xenv 管理进程级环境配置，这些信息：
//   - 在服务启动时确定
//   - 整个生命周期内不变
//   - 所有请求共享相同的值
//
// # 支持的部署类型
//
//   - LOCAL: 本地/私有化部署
//   - SAAS: SaaS 云部署
//
// # 环境变量
//
// xenv 从以下环境变量读取配置：
//
//   - DEPLOYMENT_TYPE: 部署类型，值为 "LOCAL" 或 "SAAS"（大小写不敏感）
//
// # 初始化
//
// 提供三种初始化方式：
//
//   - Init: 从环境变量读取，适用于生产环境
//   - MustInit: 同 Init，失败时 panic（仅用于 main 启动阶段）
//   - InitWith: 直接指定部署类型，适用于测试或不依赖环境变量的场景
//
// 初始化只能执行一次，重复调用返回 ErrAlreadyInitialized。
// 不提供隐式默认值：环境变量未设置返回 ErrMissingEnv，
// 空值返回 ErrEmptyEnv，非法值返回 ErrInvalidDeploymentType。
// 测试场景可使用 Reset() 重置状态（该函数仅在 go test 期间可用）。
//
// # 查询
//
//   - Type: 返回当前部署类型（未初始化返回空字符串）
//   - RequireType: 同 Type，未初始化时返回错误
//   - IsLocal/IsSaaS: 便捷判断函数（未初始化返回 false）
//
// # 线程安全
//
// 所有导出函数都是线程安全的：
//
//   - Init/MustInit/InitWith 只应在 main() 中调用一次
//   - Type/IsLocal/IsSaaS/RequireType 使用 atomic.Value 无锁读取，可高并发调用
package xenv
