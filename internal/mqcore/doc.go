// Package mqcore 提供消息队列的共享核心功能。
//
// 本包是 internal 包，仅供 xkafka 和 xpulsar 包内部使用。
// 外部用户不应直接导入此包。
//
// 依赖策略: 本包作为 MQ 族的共享内核（shared kernel），
// 依赖低层工具包（pkg/context/xctx、pkg/resilience/xretry）提取公共实现。
// 依赖链为：高层 pkg（xkafka/xpulsar）→ internal/mqcore → 低层 pkg（xctx/xretry），
// 逻辑上仍从高到低，不构成循环依赖。
//
// 主要功能：
//   - Tracer 接口：定义链路追踪的注入和提取能力
//   - OTelTracer：基于 OpenTelemetry 的 Tracer 实现
//   - NoopTracer：空实现，用于不需要追踪的场景
//   - 共享错误定义（仅含 xkafka/xpulsar 共用的 4 个错误，各自专用错误定义在各包内）
//   - Context 追踪信息合并（xctx 字段 + OTel SpanContext 双层保留）
//   - RunConsumeLoop：基于 xretry.BackoffPolicy 的消费循环工具
package mqcore
