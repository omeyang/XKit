// Package xutil 提供泛型工具函数。
//
// 设计决策: 包名 xutil 的职责边界限定为"泛型条件选择与值转换"类工具函数。
// 与具体领域绑定的 helper（如 JSON、网络、文件）应放入对应的 x* 包，
// 不应添加到本包，以避免演化为职责模糊的"上帝包"。
//
// # 功能概览
//
//   - [If]: 泛型三目运算符，编译期类型安全
//
// # 注意事项
//
//   - [If] 是 eager evaluation：trueVal 和 falseVal 在调用前均会求值，
//     不会像 if-else 那样短路。对可能 panic 的表达式（如指针解引用）请使用 if-else
//
// # 与标准库的关系
//
//   - [cmp.Or]: 返回首个非零值，用于"有值则用、否则取默认值"场景
//   - [If]: 根据布尔条件选择分支，用于需要显式条件判断的场景
//   - 两者语义不同：cmp.Or 基于零值判定，If 基于布尔条件
//
// # 相关包
//
//   - JSON 序列化：[github.com/omeyang/xkit/pkg/util/xjson]
//   - 进程信息：[github.com/omeyang/xkit/pkg/util/xproc]
//   - 系统资源限制：[github.com/omeyang/xkit/pkg/util/xsys]
package xutil
