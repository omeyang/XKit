---
package: pkg/testkit/xetcdtest
stability: Internal
coverage: 70.5%
tags: [testkit, etcd, integration-test]
---

# pkg/testkit/xetcdtest

需要真实 etcd 协议的测试可通过 `NewClient()` 创建客户端并调用返回的清理函数。底层使用嵌入式 etcd。

## 使用入口

测试文件展示嵌入式服务的启动和清理。完整签名可在仓库根目录用 `go doc ./pkg/testkit/xetcdtest` 查看。

[包说明](../../../pkg/testkit/xetcdtest/mock.go) · [使用示例](../../../tests/unit/pkg/testkit/xetcdtest/mock_test.go)

## 相关

- 包：[xetcd](../12-storage/03-xetcd.md) / [xelection](../05-distributed/03-xelection.md)
- ADR：[0008 mock 放子包](../../01-decisions/0008-mock-in-subpackage.md)
