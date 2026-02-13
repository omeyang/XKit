# 02 — 上下文模块审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 1-2** — xctx/xenv/xplatform 无内部依赖，xtenant 依赖 xctx + xplatform

## 审查范围

```
pkg/context/
├── xctx/       # 请求级 context 字段存取（追踪、租户、平台信息）
├── xenv/       # 进程级部署类型管理（LOCAL/SAAS）
├── xplatform/  # 进程级平台信息管理
└── xtenant/    # 请求级租户传播（HTTP/gRPC 中间件）
```

## 推荐 Skills

```
/code-reviewer     — 综合质量扫描
/go-style          — context 使用规范、中间件设计
/golang-patterns   — Context 传播模式、进程级单例模式
/grpc-go           — gRPC 中间件/元数据传播（xtenant）
/go-test           — 中间件测试、context 测试
```

---

## 生命周期分类

| 生命周期 | 包 | 特征 |
|---------|-----|------|
| **进程级** | xenv, xplatform | 启动时初始化，全生命周期不变，全局单例 |
| **请求级** | xctx, xtenant | 每个请求独立，通过 `context.Context` 传播 |

审查时必须验证：进程级数据不应出现在请求 context 中（除非有明确理由），请求级数据不应存储为全局变量。

---

## 模块职责与审查要点

### xctx — 请求级 Context 增强

**职责**：为 `context.Context` 提供统一的字段注入和提取方法。

**API 命名约定**：
- `WithXxx(ctx, value)` — 注入字段
- `Xxx(ctx)` — 读取字段（零值安全）
- `RequireXxx(ctx)` — 强制读取（缺失时返回错误）
- `EnsureXxx(ctx)` — 确保字段存在（缺失时自动生成默认值）

**重点审查**：
- [ ] **命名一致性**：所有字段是否严格遵循 With/Get/Require/Ensure 四套命名？是否有遗漏或不一致？
- [ ] **context key 类型安全**：key 是否使用未导出的自定义类型（避免跨包冲突）？
- [ ] **零值语义**：`Xxx(ctx)` 在 key 不存在时返回的零值是否合理？调用方是否能区分"未设置"和"设置为零值"？
- [ ] **context 膨胀**：注入的字段数量是否过多？是否存在应该用 struct 打包的一组相关字段？
- [ ] **线程安全**：`context.Value` 本身线程安全，但存入的值是否有可变状态被并发修改的风险？
- [ ] **与 xtenant 边界**：租户信息的存取是在 xctx 还是 xtenant？是否存在职责重叠？

### xenv — 进程级部署类型

**职责**：从环境变量 `DEPLOYMENT_TYPE` 读取部署类型（LOCAL/SAAS）。

**重点审查**：
- [ ] **初始化时机**：是 `init()` 自动读取还是显式 `Init()` 调用？`init()` 的隐式行为是否合适？
- [ ] **不可变性**：初始化后是否保证不可修改？测试中如何 reset（`testing` 友好性）？
- [ ] **校验**：非法环境变量值（空字符串、未知类型）的行为是否明确？
- [ ] **类型设计**：部署类型是 `string` 常量还是枚举类型（`type DeploymentType int`）？枚举更安全
- [ ] **默认值**：环境变量未设置时是否有合理默认值？默认值的选择是否安全？

### xplatform — 进程级平台信息

**职责**：管理平台信息（platform_id, has_parent 等）。

**重点审查**：
- [ ] **数据来源**：平台信息从哪里加载（环境变量？配置文件？API？）？加载失败时的行为？
- [ ] **不可变性**：与 xenv 相同，初始化后是否只读？
- [ ] **字段完整性**：平台信息结构体的字段是否与业务需求匹配？是否有冗余字段？
- [ ] **与 xctx 关系**：平台信息是否需要注入到请求 context？如果是进程级常量，为什么要放 context？

### xtenant — 请求级租户传播

**职责**：HTTP/gRPC 中间件，在服务间传播租户信息。

**重点审查**：
- [ ] **中间件链顺序**：租户中间件在 trace/auth/logging 之前还是之后？顺序是否有文档说明？
- [ ] **HTTP header 规范**：使用哪些 header 名？是否标准化（如 `X-Tenant-ID`）？大小写是否统一？
- [ ] **gRPC metadata**：metadata key 是否与 HTTP header 对应？是否支持 gRPC gateway 场景？
- [ ] **缺失租户**：请求中无租户信息时的行为？拒绝请求（返回 401/403）还是使用默认值？
- [ ] **多租户隔离**：是否有机制防止租户信息被篡改（如下游服务覆盖上游传递的租户 ID）？
- [ ] **传播完整性**：HTTP → gRPC、gRPC → HTTP、gRPC → gRPC 等跨协议场景是否全部覆盖？
- [ ] **中间件可选性**：是否支持某些路由跳过租户检查（如健康检查端点）？

---

## 跨包一致性检查

- [ ] xctx 与 xtenant 的租户字段是否使用相同的 context key？还是各自独立（可能导致数据丢失）？
- [ ] xenv 和 xplatform 的初始化模式是否一致（都用 `Init()` 或都用 `init()`，不要混用）？
- [ ] context 字段的命名（如 trace ID、tenant ID）在 xctx、xlog、xtrace 中是否统一？
- [ ] 所有中间件（xtenant, xtrace）是否遵循相同的接口签名和链式调用模式？
