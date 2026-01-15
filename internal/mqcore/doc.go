// Package mqcore 提供消息队列的共享核心功能。
//
// 本包是 internal 包，仅供 xkafka 和 xpulsar 包内部使用。
// 外部用户不应直接导入此包。
//
// 主要功能：
//   - Tracer 接口：定义链路追踪的注入和提取能力
//   - OTelTracer：基于 OpenTelemetry 的 Tracer 实现
//   - NoopTracer：空实现，用于不需要追踪的场景
//   - 共享错误定义
//   - Context 追踪信息合并
//   - RunConsumeLoop：基于 xretry.BackoffPolicy 的消费循环工具
package mqcore
