# 04 — 可观测性模块审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 1-3** — xsampling 无依赖；xmetrics → xctx；xrotate → xfile；xlog → xctx + xrotate；xtrace → xctx + xlog

## 审查范围

```
pkg/observability/
├── xsampling/  # 采样策略（比率/计数/概率/一致性）
├── xmetrics/   # 可观测性接口（Observer/Span/Attr），OTel 默认实现
├── xrotate/    # 日志文件轮转，基于 lumberjack
├── xlog/       # 结构化日志，基于 log/slog 扩展
└── xtrace/     # HTTP/gRPC 链路追踪传播中间件
```

## 推荐 Skills

```
/code-reviewer   — 综合质量扫描
/otel-go         — OpenTelemetry trace/metrics 最佳实践（核心）
/go-style        — 接口设计、中间件模式
/golang-patterns — Observer 模式、装饰器模式
/grpc-go         — gRPC 拦截器中的追踪传播
/go-test         — 可观测性组件的测试策略
```

---

## 模块内部依赖链

```
xsampling（独立）
xrotate → xfile
xmetrics → xctx
xlog → xctx, xrotate
xtrace → xctx, xlog
```

审查顺序建议：xsampling → xmetrics → xrotate → xlog → xtrace

---

## 模块职责与审查要点

### xsampling — 采样策略

**职责**：提供多种采样器（比率、计数、概率、一致性），支持跨进程一致性采样。

**重点审查**：
- [ ] **采样决策确定性**：给定相同输入（如 trace ID），一致性采样器是否总是返回相同结果？
- [ ] **概率精度**：低采样率（如 0.001%）时实际采样率是否接近目标？是否有统计验证测试？
- [ ] **并发安全**：计数采样器的计数器是否用原子操作？竞态条件？
- [ ] **接口设计**：采样器接口是否与 OTel Sampler 兼容？是否支持组合（如 AND/OR 组合多个采样策略）？
- [ ] **零值行为**：未配置采样率时默认是全采还是全不采？

### xmetrics — 可观测性接口

**职责**：定义最小化 Observer/Span/Attr 接口，默认 OTel 实现，统一指标命名。

**重点审查**：
- [ ] **接口最小化**：Observer 接口方法数量是否最少必要？是否存在"接口膨胀"？
- [ ] **Noop 实现**：是否提供 Noop Observer/Span（用于测试和无需可观测性的场景）？零开销？
- [ ] **属性键统一**：`attrSemType`, `attrResource` 等常量是否在所有使用处（xsemaphore, xlog 等）一致？
- [ ] **指标命名规范**：是否遵循 OTel 语义约定（`xkit.xxx.operation.duration` 等）？
- [ ] **Span 状态设置**：错误时是否设置 `codes.Error` + `RecordError`？成功时是否设置 `codes.Ok`？
- [ ] **指标类型选择**：Counter vs Gauge vs Histogram 的选择是否正确？Duration 用 Histogram，当前连接数用 Gauge？
- [ ] **高基数风险**：属性值是否可能有高基数（如 resource name 包含动态 ID）？是否有截断/归一化？
- [ ] **OTel SDK 依赖**：是否仅依赖 `go.opentelemetry.io/otel/api` 而非 SDK（允许使用方选择实现）？

### xrotate — 日志轮转

**职责**：Rotator 接口，基于 lumberjack 实现文件轮转。

**重点审查**：
- [ ] **接口抽象**：Rotator 接口是否足够通用（不绑定 lumberjack 特性）？
- [ ] **路径安全**：日志文件路径是否经过 xfile.SanitizePath 校验？
- [ ] **轮转触发**：按大小、按时间、按两者组合？触发条件是否可配置？
- [ ] **磁盘满处理**：磁盘空间不足时是否有 fallback（如写 stderr）？是否有告警？
- [ ] **并发写入**：多个 goroutine 同时写日志时 io.Writer 是否安全？
- [ ] **旧文件清理**：最大保留文件数/天数是否可配置？清理时是否有竞态条件？

### xlog — 结构化日志

**职责**：基于 `log/slog` 扩展，EnrichHandler 自动注入 context 字段，支持动态级别。

**重点审查**：
- [ ] **Handler 链**：EnrichHandler 是否正确包装底层 Handler？是否支持链式组合？
- [ ] **Context 字段提取**：从 context 提取哪些字段（trace ID、tenant ID 等）？是否与 xctx 定义一致？
- [ ] **动态级别**：运行时修改日志级别是否线程安全？是否支持按包/模块设置不同级别？
- [ ] **延迟求值**：`slog.LogValuer` 的使用是否正确？是否在 level 不满足时跳过求值？
- [ ] **性能**：日志热路径（每次请求）是否有不必要的内存分配？是否需要 `sync.Pool`？
- [ ] **格式化**：JSON 和 Text 格式是否都支持？生产环境默认 JSON？
- [ ] **敏感信息**：是否有机制防止敏感字段（password, token）误入日志？

### xtrace — 链路追踪

**职责**：W3C Trace Context 传播，HTTP/gRPC 中间件。

**重点审查**：
- [ ] **W3C 规范**：traceparent/tracestate 头的解析和生成是否完全符合 W3C Trace Context 规范？
- [ ] **传播方向**：是否支持双向传播（client → server 提取，server → downstream 注入）？
- [ ] **中间件接口**：HTTP 中间件签名是否与标准 `http.Handler` / `http.HandlerFunc` 兼容？
- [ ] **gRPC 拦截器**：是否同时提供 Unary 和 Stream 拦截器？Client 和 Server 端是否都有？
- [ ] **采样决策传播**：上游的采样决策是否正确传播到下游？`tracestate` 中的自定义字段？
- [ ] **无 trace 请求**：请求头中没有 trace 信息时是否自动创建新的 root span？
- [ ] **span 关闭**：中间件创建的 span 是否在请求结束时正确关闭？defer span.End()？
- [ ] **错误记录**：HTTP 5xx / gRPC 错误码是否自动记录到 span 状态？

---

## 跨包一致性检查

- [ ] xlog 提取的 context 字段列表是否与 xctx 提供的字段列表完全一致？
- [ ] xmetrics 的属性键（如 `attrResource`）在 xlog 和 xtrace 中是否复用同一常量？
- [ ] xrotate 的文件路径处理是否使用 xfile（01-util 依赖）？
- [ ] xtrace 的中间件模式是否与 xtenant（02-context）的中间件模式一致？
- [ ] 所有可观测性包的指标/日志/追踪是否使用统一的命名前缀？
- [ ] OTel SDK 版本是否在 go.mod 中统一（避免 api 和 sdk 版本不匹配）？
