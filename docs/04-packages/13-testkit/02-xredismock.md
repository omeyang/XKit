---
package: pkg/testkit/xredismock
stability: Internal
coverage: 87.0%
tags: [testkit, redis, mock]
---

# pkg/testkit/xredismock

Redis Mock 客户端。集成测试用，不对生产使用。

## 用途

- 测试需要 Redis 的 XKit 包（xsemaphore / xdlock / xcache / xlimit / xauth）
- 避免依赖外部 Redis

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/testkit/xredismock"

func TestSomething(t *testing.T) {
    mock := xredismock.New(t)
    defer mock.Close()

    client := mock.Client()
    // 使用 client 测试
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Mock` | Mock 实例 |
| `New(t)` | 启动，自动随 t.Cleanup 关闭 |
| `Client()` | 获取 redis.UniversalClient |

## 相关

- 包：使用 Redis 的所有 XKit 包
- ADR：[0008 mock 放子包](../../01-decisions/0008-mock-in-subpackage.md)
