# XKit 命名规范

> 本文档定义 XKit 项目的命名约定，遵循 Go 社区事实标准，并与项目统一规范保持一致。

---

## 设计原则

1. **简洁准确**：短而清晰，避免含糊
2. **一致性**：同类事物同一模式
3. **禁止泛化**：避免 `manager/service/handler/util/helper/common/base/data`
4. **Go 惯例**：遵循 Go 官方和社区事实标准

---

## 目录命名（模块路径片段）

| 规则 | 示例 | 说明 |
|------|------|------|
| 小写单词 | `context`, `observability`, `storage` | 领域目录使用小写 |
| `x` 前缀 | `xctx`, `xlog`, `xcache` | 包目录使用 x 前缀 |

## 包命名（package 行）

| 规则 | 示例 | 说明 |
|------|------|------|
| `x` 前缀 + 小写 | `xstr`, `xauth`, `xkv` | 统一加前缀，避免冲突 |
| 单词尽量短 | `xmetrics` 而非 `xmetricshelper` | 简短清晰 |
| 无符号 | `xlog` | 禁止下划线、短横线 |

目录到包名映射示例：

```
pkg/context/xctx       -> package xctx
pkg/observability/xlog -> package xlog
pkg/storage/xcache     -> package xcache
pkg/resilience/xretry  -> package xretry
```

## 模块路径（go.mod）

| 规则 | 示例 | 说明 |
|------|------|------|
| 允许 kebab-case | `module github.com/yourname/demo-proj` | 与目录命名一致 |

---

## 文件命名

| 规则 | 示例 | 说明 |
|------|------|------|
| snake_case + `.go` | `string_utils.go` | 源码文件 |
| 测试文件 | `string_utils_test.go` | 与被测文件一致 |
| 性能测试 | `string_utils_bench_test.go` | 基准测试 |
| 模糊测试 | `string_utils_fuzz_test.go` | Fuzz 测试 |
| 示例测试 | `example_test.go` | 示例测试 |
| 平台特定 | `file_linux.go` | 构建标签 |

### 文件组织约定

```
pkg/storage/xcache/
├── xcache.go           # 包入口，定义接口和类型
├── errors.go           # 错误定义
├── options.go          # 配置选项
├── loader.go           # Loader 公开 API
├── loader_impl.go      # Loader 内部实现
├── redis.go            # Redis 实现
├── memory.go           # Memory 实现
├── xcache_test.go      # 单元测试
├── loader_test.go      # Loader 测试
├── loader_bench_test.go # 性能测试
└── example_test.go     # 示例测试
```

---

## 类型命名

### 导出类型（公开 API）

| 规则 | 示例 | 说明 |
|------|------|------|
| PascalCase | `Tokenizer`, `RetryPolicy` | 导出类型 |
| 名词或名词短语 | `TraceInfo`, `LoaderOptions` | 描述实体 |
| 不加包名前缀 | `Tokenizer` 而非 `XstrTokenizer` | 包名已提供上下文 |

### 私有类型（内部实现）

| 规则 | 示例 | 说明 |
|------|------|------|
| camelCase | `loader`, `redisWrapper` | 首字母小写 |
| 实现类型 | `loader` 实现 `Loader` | 类型名可与接口同名（小写） |

---

## 接口命名

| 规则 | 示例 | 说明 |
|------|------|------|
| 单方法：动词 + `-er` | `Loader`, `Closer`, `Reader` | Go 惯例 |
| 多方法：名词 | `Redis`, `Memory`, `Logger` | 描述能力集合 |
| 避免 `I` 前缀 | `Logger` 而非 `ILogger` | Go 不使用匈牙利命名 |

### 接口设计示例

```go
// 单方法接口 - 使用 -er 后缀
type Loader interface {
    Load(ctx context.Context, key string, loader LoadFunc, ttl time.Duration) ([]byte, error)
}

type Unlocker func(ctx context.Context) error

// 多方法接口 - 使用名词
type Redis interface {
    Client() redis.UniversalClient
    Lock(ctx context.Context, key string, ttl time.Duration) (Unlocker, error)
    Close() error
}

type Logger interface {
    Debug(ctx context.Context, msg string, attrs ...slog.Attr)
    Info(ctx context.Context, msg string, attrs ...slog.Attr)
    Warn(ctx context.Context, msg string, attrs ...slog.Attr)
    Error(ctx context.Context, msg string, attrs ...slog.Attr)
    // ...
}
```

---

## 函数和方法命名

### 构造函数

| 模式 | 示例 | 说明 |
|------|------|------|
| `New` + 类型名 | `NewLoader()`, `NewRedis()` | 返回具体类型或接口 |
| `New` 单独使用 | `xlog.New()` | 返回 Builder |
| `Must` 前缀 | `MustInit()` | panic 而非返回 error |

### Getter/Setter

| 模式 | 示例 | 说明 |
|------|------|------|
| Context Getter 无前缀 | `TraceID(ctx)`, `TenantID(ctx)` | 从 context 提取值 |
| 接口 Getter 用 `Get` | `GetLevel()` | 接口方法明确语义 |
| Setter 用 `Set` | `SetLevel()`, `SetDefault()` | 明确修改意图 |
| 带检查的 Getter | `RequireTraceID()` | 失败时返回 error |

### Context 操作

| 模式 | 示例 | 说明 |
|------|------|------|
| `With` + 值名 | `WithTraceID()`, `WithTenantID()` | 注入值到 context |
| `Ensure` + 值名 | `EnsureTraceID()` | 确保存在（不存在则生成） |
| 值名作为 Getter | `TraceID()`, `TenantID()` | 从 context 提取 |

### 中间件

| 模式 | 示例 | 说明 |
|------|------|------|
| `HTTPMiddleware` | `xtrace.HTTPMiddleware()` | HTTP 中间件 |
| `GRPCUnaryServerInterceptor` | 同名 | gRPC 拦截器 |
| `WithOptions` 后缀 | `HTTPMiddlewareWithOptions()` | 带配置版本 |

---

## 变量和常量命名

### 错误变量

| 规则 | 示例 | 说明 |
|------|------|------|
| `Err` 前缀 | `ErrNilClient`, `ErrLockFailed` | 标准错误变量 |
| 包级别 | `var ErrNotFound = errors.New(...)` | 可导出供外部比较 |

### 常量

| 规则 | 示例 | 说明 |
|------|------|------|
| PascalCase | `HeaderTraceID`, `DeployLocal` | 导出常量 |
| camelCase | `defaultTimeout` | 私有常量 |
| 分组定义 | `const ( ... )` | 相关常量放一起 |

### Context Key

| 规则 | 示例 | 说明 |
|------|------|------|
| 私有类型 | `type ctxKey string` | 避免冲突 |
| 私有常量 | `const keyTraceID ctxKey = "trace_id"` | 包内使用 |

---

## 配置选项命名

### Functional Options 模式

```go
// Option 类型
type LoaderOption func(*LoaderOptions)

// With 前缀的选项函数
func WithSingleflight(enabled bool) LoaderOption
func WithDistributedLock(enabled bool) LoaderOption
func WithLoadTimeout(d time.Duration) LoaderOption
```

### 配置结构体

```go
// 使用 Options 后缀
type LoaderOptions struct {
    EnableSingleflight     bool
    EnableDistributedLock  bool
    LoadTimeout            time.Duration
}

// 使用 Config 后缀（外部配置）
type Config struct {
    PlatformID      string
    HasParent       bool
    UnclassRegionID string
}
```

---

## 测试命名

### 测试函数

| 模式 | 示例 | 说明 |
|------|------|------|
| `Test` + 函数名 | `TestLoad` | 基础测试 |
| `Test` + 场景 | `TestLoad_CacheMiss` | 场景测试 |
| `Test` + 类型 + 方法 | `TestLoader_Load` | 方法测试 |

### 表格驱动测试

```go
func TestLoad(t *testing.T) {
    tests := []struct {
        name    string  // 测试用例名称
        input   string  // 输入
        want    string  // 期望输出
        wantErr bool    // 是否期望错误
    }{
        {name: "empty input", input: "", want: "", wantErr: true},
        {name: "valid input", input: "foo", want: "bar", wantErr: false},
    }
    // ...
}
```

### Benchmark 和 Fuzz

| 模式 | 示例 | 说明 |
|------|------|------|
| `Benchmark` 前缀 | `BenchmarkLoad` | 性能测试 |
| `Fuzz` 前缀 | `FuzzParse` | 模糊测试 |

---

## 日志字段命名

使用 snake_case，与 JSON 输出保持一致：

```go
slog.Warn("operation failed",
    "trace_id", traceID,
    "tenant_id", tenantID,
    "error", err,
)
```

### 常用字段

| 字段名 | 用途 |
|--------|------|
| `trace_id` | 链路追踪 ID |
| `span_id` | Span ID |
| `request_id` | 请求 ID |
| `tenant_id` | 租户 ID |
| `error` | 错误信息 |
| `duration` | 耗时 |
| `component` | 组件名 |
| `operation` | 操作名 |

---

## 检查清单

新代码命名时，检查以下项目：

- [ ] 包名是否全小写，无下划线？
- [ ] 文件名是否使用小写 + 下划线？
- [ ] 导出类型是否使用 PascalCase？
- [ ] 接口命名是否遵循 `-er` 规则（单方法）？
- [ ] 错误变量是否使用 `Err` 前缀？
- [ ] 构造函数是否使用 `New` 前缀？
- [ ] Context 操作是否使用 `With`/`Ensure` 前缀？
- [ ] 配置选项是否使用 Functional Options 模式？

---
