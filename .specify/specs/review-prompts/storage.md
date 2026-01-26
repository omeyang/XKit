# Storage 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/storage/
├── xcache/       # 缓存统一封装，Redis + 内存（ristretto）
├── xmongo/       # MongoDB 包装器
├── xclickhouse/  # ClickHouse 包装器
└── xetcd/        # etcd 客户端封装

internal/
└── （相关内部实现）
```

## 模块职责

**设计原则**：
- 工厂方法创建（New / NewRedis / NewMemory）
- 底层客户端直接暴露（Client() / Conn()）
- 增值功能层叠加（健康检查、统计、慢查询检测、分页等）

**注意**：通过 Client()/Conn() 直接执行的操作不计入统计和慢查询检测。

**包职责**：
- **xcache**：Redis/Memory 缓存，Loader 提供 Cache-Aside 模式（singleflight + 分布式锁）
- **xmongo**：分页查询、批量写入、慢查询检测
- **xclickhouse**：分页查询、批量插入
- **xetcd**：简化的 KV 操作、Watch 功能

**包间关系**：
- xcache Loader 可通过 WithExternalLock 集成 xdlock
- xetcd 配置可与 xdlock 复用
