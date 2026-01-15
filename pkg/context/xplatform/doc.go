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
// # 与 xctx 包的关系
//
// xplatform 管理进程级别的平台信息（初始化一次，全局使用），
// xctx 管理请求级别的上下文信息（每个请求独立传递）。
// 典型用法是将 xplatform 的值通过 xctx.WithXxx 注入到 context 中传播。
package xplatform
