# XKit 代码审查指南

本目录包含 XKit 各模块的代码审查 prompts。

## 审查流程

1. 阅读本文件了解通用审查方法
2. 选择要审查的模块，阅读对应的模块 prompt
3. 使用工具分析代码，输出审查报告

## 模块列表

| 模块 | 文件 | 范围 |
|------|------|------|
| Context | [context.md](context.md) | xctx, xenv, xplatform, xtenant |
| Observability | [observability.md](observability.md) | xlog, xmetrics, xtrace, xsampling, xrotate |
| Resilience | [resilience.md](resilience.md) | xretry, xbreaker |
| Storage | [storage.md](storage.md) | xcache, xmongo, xclickhouse, xetcd |
| MQ | [mq.md](mq.md) | xkafka, xpulsar |
| Distributed | [distributed.md](distributed.md) | xdlock, xcron |
| Config & Utils | [config-utils.md](config-utils.md) | xconf, xfile, xpool |

---

## 通用审查要求

### 审查范围

- **同时审查 pkg 和 internal**：不要只看公开 API，要完整查看实现细节
- **追踪上下游调用**：理解函数被谁调用、调用了谁
- **查看测试代码**：评估测试质量和覆盖情况
- **检查包间依赖**：理解模块间的依赖关系

### 输出格式

- 用中文输出审查结果
- **不要直接修改代码**，只分析问题并提出改进建议
- 问题按严重程度分类：
  - 🔴 **严重**：生产故障、数据丢失、安全漏洞
  - 🟡 **中等**：可用性、性能、可维护性问题
  - 🟢 **轻微**：代码规范、文档等

### 关于新功能建议

- 可以建议新功能，但要**谨慎且有价值**
- 必须说明：为什么需要？解决什么问题？
- 避免为增加功能而增加功能
- 优先考虑简化而非添加

---

## 审查工具

以下工具可以帮助分析代码，根据需要选用：

### 调用关系分析

```bash
# go-callvis - 生成调用关系图
go install github.com/ondrajz/go-callvis@latest
go-callvis -group pkg ./pkg/xxx/yyy

# guru - 静态分析，查找调用者/被调用者
go install golang.org/x/tools/cmd/guru@latest
guru callers ./pkg/xxx/yyy.go:#offset    # 谁调用了这个函数
guru callees ./pkg/xxx/yyy.go:#offset    # 这个函数调用了谁
```

### 依赖分析

```bash
# 模块依赖图
go mod graph | grep xkit

# 包依赖
go list -f '{{.ImportPath}} -> {{.Imports}}' ./pkg/...
```

### 静态检查

```bash
# golangci-lint（项目已配置）
task lint

# go vet
go vet ./...

# staticcheck
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...
```

### 测试覆盖率

```bash
# 生成覆盖率报告
task test-cover

# 单个包覆盖率
go test -coverprofile=cover.out ./pkg/xxx/...
go tool cover -html=cover.out
```

### LSP 工具（IDE 集成）

- **Go to Definition** - 跳转到定义
- **Find References** - 查找所有引用
- **Find Implementations** - 查找接口实现
- **Call Hierarchy** - 调用层次分析

### 其他工具

```bash
# 查找未使用的代码
go install github.com/remyoudompheng/go-misc/deadcode@latest
deadcode ./...

# 检查 goroutine 泄漏风险
go install github.com/uber-go/goleak@latest
# 在测试中使用 goleak.VerifyNone(t)

# 检查竞态条件
go test -race ./...
```

---

## 审查维度（参考）

以下维度仅供参考，自行判断代码中存在的问题：

- **正确性**：逻辑是否正确，边界条件处理
- **并发安全**：竞态条件、死锁风险
- **错误处理**：错误是否正确传播和处理
- **资源管理**：连接、文件、goroutine 的生命周期
- **API 设计**：接口是否一致、易用
- **性能**：热路径性能、内存分配
- **安全**：注入攻击、权限检查等

---

## 项目背景

XKit 是 Go 工具库，供业务服务调用。Go 1.25+，K8s 原生部署。

**设计原则**：
- 工厂函数创建（New / NewXxx）
- 底层客户端暴露（Client() / Conn()）
- 增值功能层叠加
