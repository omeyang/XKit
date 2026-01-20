# 模块审查：pkg/context（上下文与身份管理）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/context/
├── xctx/       # Context 增强：注入/提取追踪、租户、平台信息
├── xenv/       # 进程级部署类型管理（LOCAL/SAAS）
├── xplatform/  # 进程级平台信息管理
└── xtenant/    # 请求级租户传播（HTTP/gRPC 中间件）
```

状态生命周期分类：
- **进程级**（xenv, xplatform）：服务启动时初始化，全生命周期不变
- **请求级**（xctx, xtenant）：每个请求独立，通过 context 传播

---

## xctx：请求上下文管理

**职责**：为 context.Context 提供标准化的字段存取。

**关键文件**：
- `identity.go` - 身份信息：platform_id, tenant_id, tenant_name
- `platform.go` - 平台信息：has_parent, unclass_region_id
- `trace.go` - 追踪信息：trace_id, span_id, request_id, trace_flags
- `deploy.go` - 部署类型：LOCAL/SAAS
- `slog_attrs.go` - slog 属性转换
- `xctx.go` - 错误定义和 contextKey 类型

**API 命名约定**：
| 前缀 | 语义 | nil ctx 行为 | 值缺失行为 |
|-----|------|-------------|-----------|
| `WithXxx` | 注入值 | ErrNilContext | N/A |
| `Xxx` | 读取值 | 零值 | 零值 |
| `RequireXxx` | 强制读取 | error | ErrMissingXxx |
| `EnsureXxx` | 确保存在 | error | 自动生成并注入 |
| `GetXxx` | 批量读取 | 空结构体 | 字段为零值 |

**哨兵错误**：ErrNilContext、ErrMissingXxx 系列、ErrInvalidDeploymentType

**Trace ID 生成**：
- trace_id: 32 hex (128-bit)，span_id: 16 hex (64-bit)，符合 W3C Trace Context
- 随机源：crypto/rand

---

## xenv：部署类型管理

**职责**：管理进程级部署类型，从环境变量 `DEPLOYMENT_TYPE` 读取。

**关键文件**：
- `deploy.go` - 核心逻辑

**API**：
```go
xenv.Init() / MustInit()     // 从环境变量初始化
xenv.IsLocal() / IsSaaS()    // 查询部署类型
xenv.Reset()                 // 测试用，重置状态
```

**同步机制**：sync.RWMutex + atomic.Bool（初始化标志）

---

## xplatform：平台信息管理

**职责**：管理进程级平台信息，通常从 AUTH 服务获取后初始化。

**关键文件**：
- `platform.go` - 核心逻辑

**API**：
```go
xplatform.Init(cfg) / MustInit(cfg)
xplatform.PlatformID() / HasParent() / UnclassRegionID()
xplatform.RequirePlatformID()  // 强制读取
xplatform.GetConfig()          // 返回配置副本
```

**Config 结构**：PlatformID（必填）、HasParent、UnclassRegionID

**同步机制**：sync.RWMutex + atomic.Bool（初始化标志）

---

## xtenant：租户传播中间件

**职责**：在 HTTP/gRPC 调用间传播租户信息。

**关键文件**：
- `http.go` - HTTP 中间件
- `grpc.go` - gRPC 拦截器
- `context.go` - 上下文操作
- `errors.go` - 错误定义

**传播机制**：
- HTTP: 通过 Header（X-Tenant-ID 等）
- gRPC: 通过 Metadata

**中间件类型**：
- 服务端：从请求中提取租户信息，注入 context
- 客户端：从 context 中提取租户信息，注入请求

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**API 一致性**：
- WithXxx/RequireXxx/EnsureXxx 系列函数行为是否符合上述约定？
- 哨兵错误是否支持 errors.Is() 检查？

**并发安全**：
- xenv/xplatform 的 Init() 是否处理并发调用？
- 初始化顺序：先写配置，后设 initialized 标志？

**ID 生成**：
- trace_id/span_id 格式是否符合 W3C 规范？
- crypto/rand 在高并发下的行为？

**中间件**：
- HTTP Header 大小写处理（HTTP/2 强制小写）
- gRPC Metadata key 是否全部小写？
- 恶意 Header 注入风险？

**测试隔离**：
- xenv.Reset() / xplatform.Reset() 是否在测试中正确使用？
- t.Cleanup() 注册清理？

**跨服务传播**：
- trace_id 在整个调用链是否保持一致？
- 缺少中间件的服务会导致链路断裂吗？
