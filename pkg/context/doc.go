// Package context 提供上下文与身份管理相关的子包。
//
// 子包列表：
//   - xctx: Context 增强，注入/提取追踪、租户、平台信息
//   - xtenant: HTTP/gRPC 租户信息中间件
//   - xplatform: 平台信息管理（平台 ID、父级关系等）
//   - xenv: 环境变量管理，部署类型检测
//
// 设计原则：
//   - 请求级上下文信息通过 context.Context 传递（xctx, xtenant）
//   - 进程级配置通过全局单例管理，初始化后不可变（xenv 部署类型、xplatform 平台信息）
//   - 提供中间件自动注入/提取，减少业务代码侵入
//   - 支持 W3C Trace Context 标准
package context
