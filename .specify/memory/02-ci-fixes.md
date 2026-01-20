# 02-CI 修复记录

本文件记录 CI/CD 相关问题的修复历史，确保相关问题不会循环出现。

---

## 修复记录 #1: Go 版本不匹配与测试数据竞争 (2026-01-16)

### 问题描述

GitHub Actions CI 运行失败，错误发生在三个方面：
1. **Go 版本不匹配**: `go.mod` 声明 `go 1.25.0`，但 CI 配置使用 `go 1.23.x`
2. **测试数据竞争**: `pkg/distributed/xcron` 包中存在 race condition
3. **安全扫描**: 因 Go 版本不匹配导致构建失败

### 根因分析

1. **Go 版本问题**:
   - `go.mod` 需要 Go 1.25.0
   - CI 配置 (`.github/workflows/ci.yml`) 中使用了旧版本 `1.23.x`
   - 依赖链中多个包（如 k8s.io、go.opentelemetry.io 等）已升级到需要 Go 1.24+/1.25+ 的版本

2. **数据竞争问题**:
   - `stats_test.go:TestScheduler_StatsIntegration` - 使用普通 `int64` 而非 `atomic.Int64`
   - `example_test.go:Example_removeJob` - 使用普通 `bool` 而非 `atomic.Bool`
   - cron 任务在并发 goroutine 中执行，对共享变量的非原子操作导致 race condition

### 修复内容

#### 1. CI 配置修复 (`.github/workflows/ci.yml`)

```yaml
# 修复前
go-version: '1.23.x'

# 修复后
go-version: '1.25.x'
```

涉及的 job:
- `lint`: 1.23.x → 1.25.x
- `test`: 1.23.x → 1.25.x
- `build` matrix: ['1.22.x', '1.23.x'] → ['1.24.x', '1.25.x']
- `security`: 1.23.x → 1.25.x

#### 2. 测试代码修复

**stats_test.go**:
```go
// 修复前
var successCount, failureCount int64
successCount++
failureCount++

// 修复后
var successCount, failureCount atomic.Int64
successCount.Add(1)
failureCount.Add(1)
```

**example_test.go**:
```go
// 修复前
var executed bool
if !executed {
    executed = true
    // ...
}

// 修复后
var executed atomic.Bool
if !executed.Swap(true) {
    // ...
}
```

### 版本约束关系

```
go.mod (go 1.25.0)
    ↓ 必须一致
CI workflows (go-version)
    ↓ 决定
依赖兼容性 (k8s, otel 等需要 1.24+)
```

**关键约束**:
- `go.mod` 中的 Go 版本是约束来源
- CI 中的 Go 版本必须 >= `go.mod` 中声明的版本
- 如需降低 Go 版本，必须同时：
  1. 修改 `go.mod` 中的版本
  2. 降级不兼容的依赖
  3. 更新 CI 配置

### 验证清单

修改完成后，必须验证：

- [ ] `go test -race ./...` 全部通过
- [ ] `golangci-lint run ./...` 无 issue
- [ ] `govulncheck ./...` 无漏洞
- [ ] CI 中所有 job 通过

### 防止回归的措施

1. **不要单独修改 CI 中的 Go 版本** - 必须与 `go.mod` 保持一致
2. **测试中使用并发原语** - 所有在 cron/scheduler 回调中的共享变量必须使用 `atomic` 或 `sync` 包
3. **升级依赖时检查 Go 版本要求** - 使用 `go mod tidy` 后检查是否需要更新 `go.mod` 中的版本

### 相关文件

- `.github/workflows/ci.yml` - CI 配置
- `go.mod` - Go 版本声明
- `pkg/distributed/xcron/stats_test.go` - 测试修复
- `pkg/distributed/xcron/example_test.go` - 示例测试修复

---

## 附录: 常见 CI 问题速查表

| 错误类型 | 可能原因 | 解决方向 |
|---------|---------|---------|
| `requires go 1.X` | go.mod 与 CI 版本不匹配 | 统一 go.mod 和 CI 中的版本 |
| `race detected` | 测试中存在并发数据竞争 | 使用 atomic/sync 原语 |
| `golangci-lint: failed` | 代码质量问题 | 查看 lint 输出并修复 |
| `govulncheck: vulnerability found` | 依赖有安全漏洞 | 升级受影响的依赖 |
