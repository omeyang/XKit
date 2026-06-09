---
title: 测试模式
tags: [concept, testing]
---

# 测试模式

## 表驱动

XKit 强制表驱动作为单元测试默认范式：

```go
func TestFoo(t *testing.T) {
    tests := []struct {
        name string
        in   string
        want string
        err  error
    }{
        {"happy", "abc", "ABC", nil},
        {"empty", "", "", ErrEmpty},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Foo(tt.in)
            if !errors.Is(err, tt.err) {
                t.Fatalf("err: got %v want %v", err, tt.err)
            }
            if got != tt.want {
                t.Fatalf("got %q want %q", got, tt.want)
            }
        })
    }
}
```

## Fuzz 测试

XKit 多个包用 Go 原生 fuzz（Go 1.18+）：
- [xelection.FuzzNewEtcdElection_Prefix](../04-packages/05-distributed/03-xelection.md)（前缀合法性）
- [xid](../04-packages/14-util/02-xid.md)（machine ID 提供器）
- [xhealth.FuzzCheckResult_MarshalJSON](../04-packages/07-lifecycle/01-xhealth.md)
- [xjson](../04-packages/14-util/03-xjson.md)（Pretty 输入边界）

## 集成测试 build tag

依赖外部中间件的测试用 `integration` build tag：

```go
//go:build integration

package xkafka_test
```

```bash
go test -tags=integration ./pkg/mq/...
```

容器编排见 [deploy/integration/README.md](../../deploy/integration/README.md)。

## goleak 全包 TestMain

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

检测包测试结束后是否还有未退出 goroutine。

## httptest 与 bufconn

- HTTP：`net/http/httptest`
- gRPC：`google.golang.org/grpc/test/bufconn`

[xetcdtest](../04-packages/13-testkit/01-xetcdtest.md) 用 bufconn dialer 传播 ctx（FG-M fix）。

## Mock 子包

按 [ADR-0008](../01-decisions/0008-mock-in-subpackage.md)：mock 放在 `<pkg>mock/` 子包，避免拖低主包覆盖率。详见 [Mock 隔离](05-mock-isolation.md)。

## Golden Files

复杂输出（如 JSON / 渲染结果）用 golden files：

```go
got, _ := json.Marshal(obj)
golden := filepath.Join("testdata", t.Name() + ".golden")
if *update {
    _ = os.WriteFile(golden, got, 0644)
}
want, _ := os.ReadFile(golden)
if !bytes.Equal(got, want) { t.Fatal(...) }
```

## 覆盖率目标

- 核心业务包 ≥ 95%
- 整体 ≥ 90%

权威数字以 `task test-cover` 为准（见 [进度追踪](../02-progress.md)）。

## benchmem

性能测试用 `go test -bench=. -benchmem` 跟踪 allocations：

```bash
task bench
```

## 相关

- 概念：[Mock 隔离](05-mock-isolation.md)、[并发安全](01-concurrency-safety.md)
- ADR：[0008 mock 放子包](../01-decisions/0008-mock-in-subpackage.md)
