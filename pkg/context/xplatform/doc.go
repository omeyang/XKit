// Package xplatform 提供平台信息的全局管理。
//
// 平台信息包括：
//   - PlatformID: 当前服务所属平台 ID（从 AUTH 服务获取）
//   - HasParent: 是否有上级平台（用于 SaaS 多级部署场景）
//   - UnclassRegionID: 未分类区域 ID
//
// # 初始化
//
// 在服务启动时调用 Init 或 MustInit 初始化。初始化后可通过
// PlatformID()、HasParent()、UnclassRegionID() 全局访问平台信息。
//
// PlatformID 校验规则：不能为空/纯空白、不能包含空白字符或控制字符、最大长度 128 字节。
// UnclassRegionID 校验规则（非空时）：纯空白归一化为空字符串（视为未设置）、不能包含空白字符或控制字符、最大长度 128 字节。
//
// # 查询
//
//   - PlatformID/HasParent/UnclassRegionID: 返回对应字段值（未初始化返回零值）
//   - RequirePlatformID: 同 PlatformID，未初始化时返回错误
//   - GetConfig: 返回完整配置副本
//
// # 与 xenv 的初始化方式差异
//
// xenv.Init() 从环境变量 DEPLOYMENT_TYPE 读取，因为部署类型是标准化的枚举值。
// xplatform.Init(cfg) 接受 Config 结构体直接传值，因为平台信息通常来自
// AUTH 服务或配置文件，非单一环境变量可表达。
//
// # 与 xctx 包的关系
//
// xplatform 管理进程级别的平台信息（初始化一次，全局使用），
// xctx 管理请求级别的上下文信息（每个请求独立传递）。
// 典型用法是将 xplatform 的值通过 xctx.WithXxx 注入到 context 中传播。
//
// # 线程安全
//
// 所有导出函数都是线程安全的：
//
//   - Init/MustInit 只应在 main() 中调用一次
//   - 读取函数使用 atomic.Pointer 实现无锁访问，可高并发调用
//   - Reset 仅用于测试，不应在生产代码中调用
package xplatform
