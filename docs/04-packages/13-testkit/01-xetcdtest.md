---
package: pkg/testkit/xetcdtest
stability: Internal
coverage: 70.5%
tags: [testkit, etcd, integration-test]
---

# pkg/testkit/xetcdtest

嵌入式 etcd server。集成测试用，不对生产使用。

## 用途

- xetcd / xelection / xdlock / xcron 等的集成测试
- 避免依赖外部 etcd 实例

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/testkit/xetcdtest"

func TestSomething(t *testing.T) {
    server := xetcdtest.NewEmbedded(t)
    defer server.Close()

    client := server.Client()
    // 使用 client 测试
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Embedded` | 嵌入式 etcd 实例 |
| `NewEmbedded(t)` | 启动，自动随 t.Cleanup 关闭 |
| `Client()` | 获取 clientv3.Client |
| `MockStore / MockIndexer` | Informer 测试桩 |

## 设计要点

- **bufconn dialer** 改用 `DialContext(ctx)` 传播 context（FG-M fix）
- **握手失败断言** 用 `assert.NoError(ctx.Err())`
- 覆盖率仅 70.5%：测试桩本身的测试较少，由调用方间接覆盖

## 相关

- 包：[xetcd](../12-storage/03-xetcd.md) / [xelection](../05-distributed/03-xelection.md)
- ADR：[0008 mock 放子包](../../01-decisions/0008-mock-in-subpackage.md)
