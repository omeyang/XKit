---
title: Singleflight 模式
tags: [pattern, concurrency, cache]
---

# Singleflight

## 问题

同 key 的并发请求各自打数据库，浪费资源 + 缓存击穿。

## 解法

`golang.org/x/sync/singleflight`：同 key 的并发 call **只有一个真正执行**，其他等结果共享。

## 模板

```go
import "golang.org/x/sync/singleflight"

var group singleflight.Group

func GetUser(ctx context.Context, id string) (*User, error) {
    val, err, _ := group.Do(id, func() (interface{}, error) {
        return loadFromDB(ctx, id)
    })
    if err != nil { return nil, err }
    return val.(*User), nil
}
```

## 配合 Cache-Aside

```go
val, err, _ := group.Do(key, func() (interface{}, error) {
    return xcache.GetOrLoad(ctx, c, key, ttl, loader)
})
```

热点 key 失效瞬间也只有一次回源。

## 关键陷阱

### 1. 共享错误

若回源错误是临时性的（如超时），所有等待者都拿到这个错误。解决：在 loader 内部决策（如返 cached stale）。

### 2. 慢回源放大延迟

第一个等待者拖累所有等待者。**用 `DoChan` + ctx 超时**：

```go
ch := group.DoChan(key, loader)
select {
case res := <-ch:
    return res.Val, res.Err
case <-ctx.Done():
    return nil, ctx.Err()
}
```

### 3. ctx 取消

`singleflight.Do` 不接受 ctx。loader 内部要自己用 ctx。

## XKit 用例

[xauth](../04-packages/01-business/01-xauth.md) 双层缓存 + singleflight 是经典组合。

## 相关

- 模式：[Cache-Aside](02-cache-aside.md)
- 包：[xauth](../04-packages/01-business/01-xauth.md)、[xcache](../04-packages/12-storage/01-xcache.md)
