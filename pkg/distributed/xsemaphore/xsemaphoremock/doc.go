// Package xsemaphoremock 提供 [xsemaphore.Semaphore] 与 [xsemaphore.Permit] 接口
// 的 GoMock 实现，供下游业务代码在单元测试中替换真实信号量。
//
// # 为什么存在
//
// 本仓库内部不依赖此包——xsemaphore 自身的单元测试直接测试真实实现，无需 mock。
// 此包专为 import 了 [xsemaphore.Semaphore] / [xsemaphore.Permit] 接口的下游业务
// 代码而存在：
//   - 业务方写单元测试时直接 import xsemaphoremock，避免每个下游仓库各自重复维护
//     mockgen 命令与生成产物
//   - 与 xsemaphore 接口同 module 共生，接口签名变更后 go:generate 一次刷新，所有
//     下游 import 同步更新
//
// 命名与放置约定：
//   - 包名 xsemaphoremock 而非 xsemaphoretest，明确语义是接口级 GoMock 桩
//   - 路径放在 pkg/distributed/xsemaphore/xsemaphoremock/ 子目录而非 pkg/testkit/，
//     因为它与 xsemaphore 接口紧耦合，搬到 testkit 会形成 testkit → xsemaphore
//     的反向依赖
//
// 与 [pkg/testkit/xredismock] 的语义区别：xredismock 提供基于 miniredis 的行为级
// Redis 测试实例（本仓库内部 xcache / xdlock(Redis) / xlimit 等测试均依赖）；
// xsemaphoremock 是 GoMock 接口桩，只面向下游业务代码，不被本仓库内部 import。
//
// # 维护
//
// mock 源自 [xsemaphore] 包接口，由 mockgen 自动生成。生成命令保存于
// pkg/distributed/xsemaphore/semaphore.go 顶部的 //go:generate 指令；重新生成请在
// 仓库根目录执行 go generate ./pkg/distributed/xsemaphore/...
//
// # 不要删
//
// 即便 grep "xsemaphoremock" 显示本仓库 0 处 import，此包仍是对外 API 的一部分，
// 删除会破坏下游业务代码的测试 build。
package xsemaphoremock
