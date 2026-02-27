// Package storageopt 提供 storage 子包共享的配置选项和工具函数。
//
// 本包是 internal 包，仅供 pkg/storage 下的子包（xmongo、xclickhouse 等）使用。
// 外部用户不应直接导入此包。
//
// 依赖策略: 本包作为 storage 族的共享内核（shared kernel），
// 依赖低层工具包（pkg/util/xpool）提取公共实现。
// 依赖链为：高层 pkg（xmongo/xclickhouse）→ internal/storageopt → 低层 pkg（xpool），
// 逻辑上仍从高到低，不构成循环依赖。
//
// 主要功能：
//   - 健康检查配置常量
//   - 分页参数验证函数
//   - 慢查询检测器（支持同步/异步钩子）
//   - 统计计数器（HealthCounter、SlowQueryCounter 供所有存储包使用；
//     QueryCounter 仅 xclickhouse 使用，详见 stats.go 设计决策注释）
package storageopt
