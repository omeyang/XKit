# Observability 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/observability/
├── xlog/       # 结构化日志，基于 log/slog 扩展
├── xmetrics/   # 可观测性接口（Observer/Span/Attr），OTel 默认实现
├── xtrace/     # HTTP/gRPC 链路追踪传播中间件
├── xsampling/  # 采样策略（比率/计数/概率/一致性）
└── xrotate/    # 日志文件轮转，基于 lumberjack

internal/
└── （相关内部实现）
```

## 模块职责

**包间关系**：
- xlog 使用 xctx 提取追踪信息，使用 xrotate 进行轮转
- xmetrics 提供 Observer 接口，其他包可依赖
- xtrace 使用 xctx 存取追踪信息

**包职责**：
- **xlog**：EnrichHandler 自动从 context 注入字段，支持动态级别、延迟求值、日志轮转
- **xmetrics**：定义最小化 Observer/Span 接口，默认 OTel 实现，统一指标命名 `xkit.operation.*`
- **xtrace**：W3C Trace Context 传播，支持 traceparent/tracestate
- **xsampling**：多种采样器，KeyBasedSampler 提供跨进程一致性采样
- **xrotate**：Rotator 接口，基于 lumberjack 实现
