// Package rediscompat 提供 Redis 代理兼容性检测和脚本模式管理。
//
// 本包是 internal 包，仅供 xsemaphore、xcron、xcache、xdlock 包内部使用。
// 外部用户不应直接导入此包。
//
// 依赖策略: 本包作为分布式组件的共享内核（shared kernel），
// 提供 Redis 代理环境下的脚本兼容性探测能力。
// 依赖链为：高层 pkg（xsemaphore/xcron/xcache/xdlock）→ internal/rediscompat → go-redis/v9，
// 逻辑上仍从高到低，不构成循环依赖。
//
// # 背景
//
// 线上 Redis 基础设施混合部署：部分是 Redis Cluster 直连，部分通过代理（如 Predixy）。
// 部分代理对 EVAL/EVALSHA/MULTI/EXEC/WATCH 命令返回权限错误或不支持错误。
// 本包提供自动探测和手动指定两种方式，让上层包在代理环境中用基础命令替代 Lua 脚本。
//
// # 主要功能
//
//   - ScriptMode 类型：Auto/Lua/Compat 三种模式
//   - DetectScriptMode：通过 EVAL "return 1" 0 探测 Redis 是否支持 Lua 脚本
//   - IsScriptUnsupportedError：识别代理返回的脚本不支持错误
package rediscompat
