# 测试标准

## 核心原则

1. **测试驱动开发（TDD）** - SHOULD 遵循"红-绿-重构"流程
2. **高覆盖率** - 核心业务 MUST ≥95%,整体 MUST ≥90%
3. **正确使用 Mock** - SHOULD Mock 外部依赖,MUST NOT Mock 核心业务逻辑
4. **测试独立** - 测试 MUST 互不依赖,MUST 可并行运行
5. **测试清晰** - 测试 SHOULD 作为文档,MUST 一看就懂

---

## 测试驱动开发（TDD）

### TDD 三阶段

```
红（Red）→ 绿（Green）→ 重构（Refactor）
```

1. **红（Red）**：MUST 先写测试,运行失败
2. **绿（Green）**：MUST 写最简实现,让测试通过
3. **重构（Refactor）**：SHOULD 优化代码,MUST 保持测试通过

### TDD 实践流程

#### 阶段 1：编写测试（红）

```go
func TestFindByIP_Success(t *testing.T) {
    repo := NewAssetRepository(mockDB)
    asset, err := repo.FindByIP(context.Background(), "192.168.1.1")
    assert.NoError(t, err)
    assert.Equal(t, "001", asset.ID)
}
```

#### 阶段 2：实现功能（绿）

```go
func (r *AssetRepository) FindByIP(ctx context.Context, ip string) (*Asset, error) {
    var asset Asset
    err := r.db.Collection("assets").FindOne(ctx, bson.M{"ip": ip}).Decode(&asset)
    return &asset, err
}
```

#### 阶段 3：重构优化（重构）

```go
func (r *AssetRepository) FindByIP(ctx context.Context, ip string) (*Asset, error) {
    var asset Asset
    err := r.db.Collection("assets").FindOne(ctx, bson.M{"ip": ip}).Decode(&asset)
    if err != nil {
        return nil, fmt.Errorf("find asset by ip %s failed: %w", ip, err)
    }
    return &asset, nil
}
```

### TDD 优势

**SHOULD 带来以下好处**:
- 设计先行：接口优先于实现
- 快速反馈：立即发现代码缺陷
- 高覆盖率：测试自然覆盖所有代码路径
- 易于重构：测试保护代码变更
- 减少缺陷：边界条件在编码时即考虑

### TDD 常见问题

**MUST NOT 出现以下误区**:

| 误区 | 问题 | 改进方法 |
|------|------|---------|
| 只写测试不重构 | 代码质量下降 | MUST 重构实现代码 |
| 一次写完所有测试 | 偏离 TDD 流程 | SHOULD 逐个测试编写 |
| 测试过于复杂 | 维护成本高 | SHOULD 保持测试简单 |
| 为覆盖率而测试 | 测试无实际意义 | MUST 测试行为而非代码行 |

---

## 测试覆盖率要求

### 覆盖率标准

| 代码类型 | 最低覆盖率 | 要求级别 |
|---------|----------|---------|
| **核心业务逻辑** | 95% | MUST |
| **API 层** | 90% | MUST |
| **工具函数** | 90% | SHOULD |
| **数据模型** | 80% | SHOULD |
| **配置管理** | 70% | SHOULD |
| **整体代码** | 90% | MUST |

### MAY 不覆盖的代码

- `main.go` 入口文件
- 自动生成的代码（protobuf、mock）
- 简单的 getter/setter
- 已 Mock 的外部依赖

### 查看覆盖率

```bash
# 生成覆盖率报告
go test ./... -cover -coverprofile=coverage.out

# 查看整体覆盖率
go tool cover -func=coverage.out

# 生成 HTML 报告
go tool cover -html=coverage.out -o coverage.html

# 查看未覆盖代码
go tool cover -func=coverage.out | grep -v "100.0%"
```

### 提高覆盖率的策略

**SHOULD 遵循以下优先级**:

1. **优先覆盖核心逻辑** - 资产匹配、归属判定等核心算法 MUST 优先
2. **补充边界条件** - MUST 测试空值、nil、边界值、异常情况
3. **增加集成测试** - SHOULD 覆盖多模块协作场景
4. **避免无意义测试** - MUST NOT 为覆盖率而编写无用测试

---

## Mock 策略

### Mock 使用原则

#### SHOULD Mock 的场景

**外部依赖（数据库、网络、消息队列、时间）MUST Mock**:

```go
// Mock 数据库
type MockAssetRepository struct {
    mock.Mock
}

func (m *MockAssetRepository) FindByIP(ctx context.Context, ip string) (*Asset, error) {
    args := m.Called(ctx, ip)
    return args.Get(0).(*Asset), args.Error(1)
}
```

**时间相关操作 SHOULD Mock**:

```go
type MockTimeProvider struct {
    mock.Mock
}

func (m *MockTimeProvider) Now() time.Time {
    return m.Called().Get(0).(time.Time)
}
```

#### MUST NOT Mock 的场景

**核心业务逻辑 MUST NOT Mock**:

```go
// ❌ 错误：Mock 核心业务逻辑
mockMatcher := new(MockAssetMatcher)
mockMatcher.On("Match", ...).Return(expectedBranch, nil)

// ✅ 正确：测试真实业务逻辑
mockRepo := new(MockAssetRepository)  // 只 Mock 外部依赖
matcher := NewAssetMatcher(mockRepo)   // 使用真实业务逻辑
branch, err := matcher.Match(ctx, criteria)
```

**简单数据结构 MUST NOT Mock**:

```go
// ❌ 错误：Mock 简单结构体
type MockAsset struct { mock.Mock }

// ✅ 正确：直接使用真实结构体
asset := &Asset{ID: "001", IP: "192.168.1.1"}
```

**纯函数 MUST NOT Mock**:

```go
// ❌ 错误：Mock 纯函数
mockValidator.On("ValidateIP", "192.168.1.1").Return(true)

// ✅ 正确：直接调用真实函数
isValid := ValidateIP("192.168.1.1")
```

### 过度 Mock 的危害

**MUST 避免以下问题**:

| 问题 | 影响 |
|------|------|
| 测试变成测 Mock | 未验证真实逻辑,测试失去意义 |
| 维护成本高 | 代码变更需同步修改大量 Mock |
| 假阳性 | 测试通过但实际代码有缺陷 |

### Mock 最佳实践

#### 使用接口 Mock

```go
// 定义接口
type AssetRepository interface {
    FindByIP(ctx context.Context, ip string) (*Asset, error)
}

// Mock 实现接口
type MockAssetRepository struct { mock.Mock }

func (m *MockAssetRepository) FindByIP(ctx context.Context, ip string) (*Asset, error) {
    args := m.Called(ctx, ip)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Asset), args.Error(1)
}
```

#### 使用 testify/mock

```go
func TestAssetService_Query(t *testing.T) {
    mockRepo := new(MockAssetRepository)
    mockRepo.On("FindByIP", mock.Anything, "192.168.1.1").
        Return(&Asset{ID: "001"}, nil)

    service := NewAssetService(mockRepo)
    asset, err := service.Query(context.Background(), "192.168.1.1")

    assert.NoError(t, err)
    assert.Equal(t, "001", asset.ID)
    mockRepo.AssertExpectations(t)
}
```

#### 使用表驱动测试

```go
func TestAssetValidator_Validate(t *testing.T) {
    tests := []struct {
        name    string
        asset   *Asset
        wantErr bool
    }{
        {"valid asset", &Asset{ID: "001", IP: "192.168.1.1"}, false},
        {"empty ID", &Asset{IP: "192.168.1.1"}, true},
        {"invalid IP", &Asset{ID: "001", IP: "invalid"}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateAsset(tt.asset)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### 使用内存数据库替代 Mock

```go
func TestAssetRepository_Integration(t *testing.T) {
    mongoContainer := startMongoContainer(t)
    defer mongoContainer.Terminate(context.Background())

    client := connectToMongo(t, mongoContainer.URI())
    repo := NewAssetRepository(client)

    asset := &Asset{ID: "001", IP: "192.168.1.1"}
    err := repo.Create(context.Background(), asset)
    assert.NoError(t, err)

    found, err := repo.FindByIP(context.Background(), "192.168.1.1")
    assert.NoError(t, err)
    assert.Equal(t, asset.ID, found.ID)
}
```

---

## 单元测试规范

### 测试文件组织

**MUST 遵循以下结构**:

```
internal/belong/
├── belong.go           # 业务代码
├── belong_test.go      # 单元测试
├── matcher.go
├── matcher_test.go
└── testdata/           # 测试数据
    ├── assets.json
    └── branches.json
```

### 测试函数命名

**MUST 使用以下格式**:

```go
// 格式：Test<FunctionName>_<Scenario>
func TestFindByIP_Success(t *testing.T) { ... }
func TestFindByIP_NotFound(t *testing.T) { ... }
func TestFindByIP_DatabaseError(t *testing.T) { ... }
```

### 测试结构（Given-When-Then）

**SHOULD 使用 Given-When-Then 结构**:

```go
func TestAssetMatcher_Match_Success(t *testing.T) {
    // Given（准备测试数据和依赖）
    mockRepo := new(MockAssetRepository)
    mockRepo.On("FindByIP", mock.Anything, "192.168.1.1").
        Return(&Asset{ID: "001", BranchID: "branch-01"}, nil)
    matcher := NewAssetMatcher(mockRepo)

    // When（执行被测试的代码）
    branch, err := matcher.Match(context.Background(), &Criteria{IP: "192.168.1.1"})

    // Then（验证结果）
    assert.NoError(t, err)
    assert.Equal(t, "branch-01", branch.ID)
    mockRepo.AssertExpectations(t)
}
```

### 测试边界条件

**MUST 覆盖边界条件**:

```go
func TestDivide_EdgeCases(t *testing.T) {
    tests := []struct {
        name    string
        a, b    int
        want    int
        wantErr bool
    }{
        {"normal", 10, 2, 5, false},
        {"divide by zero", 10, 0, 0, true},
        {"negative numbers", -10, 2, -5, false},
        {"result is zero", 0, 5, 0, false},
        {"max int", math.MaxInt64, 1, math.MaxInt64, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Divide(tt.a, tt.b)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

---

## 集成测试规范

### 集成测试场景

**SHOULD 在以下场景编写集成测试**:
- 多个模块协作
- 涉及真实数据库操作
- 涉及消息队列
- 端到端业务流程

### 集成测试示例

```go
// +build integration

func TestAssetQueryFlow_Integration(t *testing.T) {
    env := setupTestEnvironment(t)
    defer env.Teardown()

    asset := &Asset{ID: "001", IP: "192.168.1.1", BranchID: "branch-01"}
    err := env.AssetRepo.Create(context.Background(), asset)
    require.NoError(t, err)

    request := &AssetQueryByIpArgs{Ip: "192.168.1.1"}
    response, err := env.IdentifierService.AssetQueryByIp(context.Background(), request)

    assert.NoError(t, err)
    assert.Equal(t, "001", response.AssetId)
}
```

### 运行集成测试

```bash
# 只运行单元测试
go test ./... -short

# 运行包括集成测试
go test ./... -tags=integration

# 运行特定集成测试
go test ./internal/belong -tags=integration -run TestAssetQueryFlow
```

---

## 性能测试（Benchmark）

### Benchmark 场景

**SHOULD 在以下场景编写性能测试**:
- 核心算法
- 频繁调用的函数
- 性能敏感的操作

### Benchmark 示例

```go
func BenchmarkAssetMatcher_Match(b *testing.B) {
    mockRepo := new(MockAssetRepository)
    mockRepo.On("FindByIP", mock.Anything, mock.Anything).
        Return(&Asset{ID: "001", BranchID: "branch-01"}, nil)

    matcher := NewAssetMatcher(mockRepo)
    criteria := &Criteria{IP: "192.168.1.1"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = matcher.Match(context.Background(), criteria)
    }
}
```

### 运行性能测试

```bash
# 运行所有 Benchmark
go test -bench=. ./...

# 查看内存分配
go test -bench=. -benchmem ./...

# 生成性能报告
go test -bench=. -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof
```

---

## 测试工具

### 断言库

**SHOULD 使用 testify/assert**:

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// assert：失败后继续执行
assert.Equal(t, expected, actual)

// require：失败后立即终止
require.NoError(t, err)
```

### Mock 库

**SHOULD 使用 testify/mock**:

```go
import "github.com/stretchr/testify/mock"

type MockRepository struct { mock.Mock }

func (m *MockRepository) FindByIP(ctx context.Context, ip string) (*Asset, error) {
    args := m.Called(ctx, ip)
    return args.Get(0).(*Asset), args.Error(1)
}
```

---

## 检查清单

### 测试编写检查
- [ ] SHOULD 遵循 TDD 流程（红 → 绿 → 重构）
- [ ] MUST 测试覆盖率达标（核心 ≥95%,整体 ≥90%）
- [ ] SHOULD 使用 Given-When-Then 结构
- [ ] MUST 测试命名清晰（Test<Function>_<Scenario>）
- [ ] MUST 边界条件有覆盖

### Mock 使用检查
- [ ] SHOULD 只 Mock 外部依赖
- [ ] MUST NOT Mock 核心业务逻辑
- [ ] MUST NOT Mock 简单结构体
- [ ] SHOULD 验证 Mock 调用（AssertExpectations）

### 测试质量检查
- [ ] MUST 测试独立（可并行运行）
- [ ] SHOULD 测试快速（单元测试 <1s）
- [ ] MUST 测试清晰（一看就懂）
- [ ] MUST NOT 有 flaky 测试

### 运行检查
- [ ] `go test ./...` MUST 全部通过
- [ ] `go test ./... -cover` MUST 覆盖率达标
- [ ] `go test -bench=.` MUST 性能无退化
- [ ] 集成测试 SHOULD 通过

---

## 参考资料

**官方文档**:
- Go Testing 官方文档：https://pkg.go.dev/testing
- Go Benchmark 指南：https://go.dev/blog/benchmarks
- 表驱动测试：https://go.dev/wiki/TableDrivenTests

**测试框架**:
- testify/assert：https://github.com/stretchr/testify
- testify/mock：https://github.com/stretchr/testify#mock-package
- go.uber.org/mock：https://github.com/uber-go/mock

**社区资源**:
- Effective Go（Testing）：https://go.dev/doc/effective_go#testing
- Uber Go Style Guide（Testing）：https://github.com/uber-go/guide/blob/master/style.md#test-tables
- Google Go Style Guide：https://google.github.io/styleguide/go/
