# 设计准则

## 核心原则

1. **模块化设计** - 单一职责、低耦合、高内聚
2. **面向接口编程** - 依赖抽象而非具体实现
3. **分层清晰** - 严格遵守分层架构
4. **简单优先** - 不过度设计、不引入复杂 DDD 概念

---

## 分层架构

### 四层架构（go-idt）

```
┌─────────────────────────────────────┐
│  API Layer (api/)                   │  ← HTTP/gRPC 接口定义
├─────────────────────────────────────┤
│  Service Layer (internal/belong/)   │  ← 业务逻辑实现
├─────────────────────────────────────┤
│  Model Layer (internal/model/)      │  ← 数据模型与持久化
├─────────────────────────────────────┤
│  Infrastructure (eventchange, idt)  │  ← 基础设施（消息队列、配置等）
└─────────────────────────────────────┘
```

### 分层职责

**API Layer**
- 接收请求、参数验证
- 调用 Service 层
- 返回响应、错误处理

**Service Layer**
- 业务逻辑实现
- 调用 Model 层
- 不涉及具体存储细节

**Model Layer**
- 数据模型定义
- CRUD 操作
- 缓存管理

**Infrastructure Layer**
- 消息队列
- 配置管理
- 日志、监控

### 禁止跨层调用

❌ **禁止**：API 直接访问 Model
❌ **禁止**：Service 直接访问基础设施细节
✅ **允许**：各层通过接口依赖

---

## 模块化设计

### 单一职责

每个模块只做一件事。

**✅ 正确**
```
internal/belong/          # 资产归属判定
internal/model/asset/     # 资产数据模型
internal/eventchange/     # 事件处理
```

**❌ 错误**
```
internal/service/         # 什么 service？职责不清
internal/common/          # 大杂烩
```

### 模块边界

- 模块间通过接口通信
- 模块内部实现细节不暴露
- 避免循环依赖

---

## 面向接口编程

### 依赖抽象

业务逻辑依赖接口，而非具体实现。

```go
// ✅ 定义接口
type AssetRepository interface {
    FindByIP(ctx context.Context, ip string) (*Asset, error)
    Create(ctx context.Context, asset *Asset) error
}

// ✅ 业务逻辑依赖接口
type AssetService struct {
    repo AssetRepository  // 依赖抽象
}

// ✅ 可以有多种实现
type MongoAssetRepo struct { ... }
type RedisAssetRepo struct { ... }
```

### 接口设计原则

- 接口应该小而专注（单一职责）
- 接口应该稳定（向后兼容）
- 只在需要多种实现时才定义接口

---

## 避免过度设计

### 不引入复杂 DDD 概念

❌ **不使用**：聚合根、值对象、领域事件、限界上下文等术语
✅ **使用**：清晰的模块划分、简单的数据模型

### 何时抽象

- ✅ 已经有 ≥2 种实现
- ✅ 明确未来会有多种实现
- ❌ 只有1种实现且未来不会变

### YAGNI 原则

You Aren't Gonna Need It - 不要实现还不需要的功能。

---

## 代码组织

### 包结构

```
internal/
├── belong/           # 业务逻辑：资产归属判定
│   ├── belong.go         # 主逻辑
│   ├── matcher.go        # 匹配器
│   ├── locator.go        # 定位器
│   └── belong_test.go    # 测试
│
├── model/            # 数据模型
│   ├── asset/            # 资产模型
│   ├── branch/           # 资产组模型
│   └── config/           # 配置模型
│
└── eventchange/      # 基础设施：事件处理
```

### 文件组织

- 相关功能放在同一个包
- 每个文件 ≤500 行
- 测试文件与源文件同目录

---

## 依赖管理

### 依赖方向

```
API → Service → Model → Infrastructure
```

**规则**：
- 上层依赖下层
- 下层不依赖上层
- 同层之间通过接口依赖

### 循环依赖

**禁止循环依赖**。如果出现：
1. 提取公共接口到独立包
2. 合并职责相近的模块
3. 使用依赖注入

---

## 并发设计

### Goroutine 管理

- 使用 `context.Context` 控制生命周期
- 使用 `errgroup` 管理并发任务
- 使用 `ants` 限制 Goroutine 数量

### Channel 规则

- 发送方负责关闭 Channel
- 接收方检查 Channel 是否关闭
- 避免在多个 Goroutine 中关闭同一个 Channel

### 共享数据

- 优先使用 Channel 通信
- 必要时使用 `sync.Mutex` 加锁
- 使用 `sync.Map` 处理并发读写

---

## 错误处理

### 错误传播

```go
// ✅ 使用 %w 包装错误
return fmt.Errorf("query asset failed: %w", err)

// ✅ 添加上下文
return fmt.Errorf("query asset by ip %s failed: %w", ip, err)
```

### 错误判断

```go
// ✅ 使用 errors.Is
if errors.Is(err, ErrNotFound) { ... }

// ✅ 使用 errors.As
var apiErr *APIError
if errors.As(err, &apiErr) { ... }
```

### 日志记录

- Error 级别：记录错误堆栈
- Warn 级别：记录业务上下文
- Info 级别：记录关键操作

---

## 性能设计

### 缓存策略

- 启动时加载全量数据（热数据）
- 使用 `sync.Map` 或 `ristretto` 缓存
- 定时刷新缓存

### 数据库优化

- 使用索引加速查询
- 批量操作减少网络开销
- 使用投影减少数据传输

### 并发优化

- 使用 Goroutine 池
- 避免过度并发（限流）
- 使用 `sync.Pool` 复用对象

---

## 检查清单

- [ ] 遵循四层架构（API、Service、Model、Infrastructure）
- [ ] 没有跨层调用
- [ ] 模块职责单一、边界清晰
- [ ] 业务逻辑依赖接口而非实现
- [ ] 没有循环依赖
- [ ] 没有引入复杂 DDD 概念
- [ ] Goroutine 有生命周期管理
- [ ] 共享数据有并发保护
- [ ] 错误使用 `%w` 包装并记录上下文
