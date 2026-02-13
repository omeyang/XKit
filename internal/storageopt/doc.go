// Package storageopt 提供 storage 子包共享的配置选项和工具函数。
//
// 本包是 internal 包，仅供 pkg/storage 下的子包（xmongo、xclickhouse 等）使用。
// 外部用户不应直接导入此包。
//
// 主要功能：
//   - 健康检查配置常量
//   - 分页参数验证函数
//   - 慢查询检测器（支持同步/异步钩子）
//   - 统计计数器（HealthCounter、SlowQueryCounter、QueryCounter）
package storageopt
