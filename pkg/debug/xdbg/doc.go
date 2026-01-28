// Package xdbg 提供运行时调试服务，支持在生产环境中进行安全的动态调试。
//
// # 概述
//
// xdbg 是一个轻量级的运行时调试模块，专为 Kubernetes 环境设计。它提供以下核心能力：
//
//   - 动态修改日志级别
//   - 采集 CPU/Memory profile
//   - 查看 goroutine 堆栈
//   - 查看熔断器/限流器状态
//
// # 安全设计
//
// xdbg 采用安全优先设计：
//
//   - 仅支持 Unix Socket，不暴露网络端口
//   - 文件权限 0600，仅 owner 可访问
//   - 通过 SO_PEERCRED 获取调用者身份
//   - 所有操作记录审计日志
//   - 支持自动关闭（默认 5 分钟）
//
// # 触发方式
//
// 调试服务采用 Toggle 语义：SIGUSR1 信号切换服务状态（启用↔禁用）。
// 支持两种触发方式（都需要 kubectl exec 进入 Pod）：
//
//  1. 信号触发：kill -SIGUSR1 1
//  2. 命令触发：xdbgctl toggle（或 xdbgctl toggle --name myapp）
//
// 注意：
//   - SIGUSR1 信号触发的是 toggle 操作，而非单纯的 enable
//   - Socket 文件发现需要调试服务已启用（Socket 已创建）
//   - 若服务未启用，需使用 --pid 或 --name 参数指定目标进程
//
// # 自定义命令
//
// 可以注册自定义调试命令。
//
// # xkit 集成
//
// 通过 Option 注入 xkit 组件，启用对应的调试命令。
//
// # 客户端工具
//
// xdbgctl 是配套的客户端工具，支持单命令模式和交互模式。
package xdbg
