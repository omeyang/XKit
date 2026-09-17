---
title: Mock 隔离
tags: [concept, testing, mock]
related:
  - ../01-decisions/0008-mock-in-subpackage.md
---

# Mock 隔离

## 规则

**Mock 代码放在 `<pkg>mock/` 子包**，不与主包同包。

## 反例

```
pkg/distributed/xsemaphore/
├── semaphore.go
├── semaphore_test.go
├── mock.go             ← bad: mock 与主包同包
└── doc.go
```

问题：
- mock.go 的源码量算入主包行数
- 覆盖率工具看到的是"主包 + mock 一起"
- mock 通常仅在测试时用，但生产 build 也会包含

## 正例

```
pkg/distributed/xsemaphore/
├── semaphore.go
├── semaphore_test.go
├── doc.go
└── xsemaphoremock/        ← 子包
    ├── mock.go            （mockgen 生成）
    └── doc.go
```

- 主包覆盖率 = 仅 `semaphore.go` 的覆盖
- mock 子包独立计算（通常很低，因为是生成代码，无 mock 自己的测试）

## XKit 中的 mock 位置

| 主包 | mock 子包 |
|---|---|
| [xsemaphore](../04-packages/05-distributed/04-xsemaphore.md) | `xsemaphore/xsemaphoremock` |
| 其他（testkit 整包）| [xetcdtest](../04-packages/13-testkit/01-xetcdtest.md)、[xredismock](../04-packages/13-testkit/02-xredismock.md) |

## 生成方式

mockgen：
```bash
go install go.uber.org/mock/mockgen@latest
mockgen -source=semaphore.go -destination=xsemaphoremock/mock.go -package=xsemaphoremock
```

## 业务方使用

```go
import "github.com/omeyang/xkit/pkg/distributed/xsemaphore/xsemaphoremock"

ctrl := gomock.NewController(t)
mock := xsemaphoremock.NewMockSemaphore(ctrl)
mock.EXPECT().Acquire(...).Return(nil, errors.New("test"))

svc := NewMyService(mock)
```

## 相关

- ADR：[0008 mock 放子包](../01-decisions/0008-mock-in-subpackage.md)
- 概念：[接口由使用方定义](04-interface-by-consumer.md)
- 包：[testkit MOC](../04-packages/13-testkit/00-index.md)
