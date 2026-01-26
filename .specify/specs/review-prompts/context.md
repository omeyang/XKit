# Context 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/context/
├── xctx/       # Context 增强：注入/提取追踪、租户、平台信息
├── xenv/       # 进程级部署类型管理（LOCAL/SAAS）
├── xplatform/  # 进程级平台信息管理
└── xtenant/    # 请求级租户传播（HTTP/gRPC 中间件）

internal/
└── （相关内部实现）
```

## 模块职责

**生命周期分类**：
- **进程级**（xenv, xplatform）：服务启动时初始化，全生命周期不变
- **请求级**（xctx, xtenant）：每个请求独立，通过 context 传播

**包职责**：
- **xctx**：请求级 context 字段存取，API 命名约定：WithXxx（注入）、Xxx（读取）、RequireXxx（强制读取）、EnsureXxx（确保存在）
- **xenv**：从环境变量 `DEPLOYMENT_TYPE` 读取部署类型
- **xplatform**：管理平台信息（platform_id, has_parent 等）
- **xtenant**：HTTP/gRPC 中间件，在服务间传播租户信息
