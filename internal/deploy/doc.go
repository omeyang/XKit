// Package deploy 提供部署类型的共享定义。
//
// 此包定义了 Type 类型及其方法，供 xctx 和 xenv 包共享使用。
// 这避免了两个包中重复的类型定义和方法实现。
//
// 用途区分：
//   - xctx.DeploymentType: 请求级 context 传播（类型别名 deploy.Type）
//   - xenv.DeploymentType: 进程级环境配置（类型别名 deploy.Type）
//
// 两者底层使用相同的 deploy.Type 定义。
package deploy
