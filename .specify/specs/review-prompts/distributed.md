# Distributed 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/distributed/
├── xcron/    # 分布式定时任务，基于 robfig/cron/v3
└── xdlock/   # 分布式锁，支持 etcd 和 Redis (Redlock)

internal/
└── （相关内部实现）
```

## 模块职责

**设计原则**：
- 工厂函数：NewEtcdFactory, NewRedisFactory
- 底层暴露：Session()/Redsync()/Mutex()/Cron()
- 增值功能：健康检查、统一错误处理、自动续期

**xdlock 后端差异**：
| 特性 | etcd | Redis (redsync) |
|------|------|-----------------|
| 续期方式 | 自动（Session） | 手动（Extend） |
| 多节点支持 | 原生（etcd 集群） | Redlock 算法 |

**xcron Locker**：
| Locker | 适用场景 | 底层实现 |
|--------|---------|---------|
| NoopLocker | 单副本 | 无锁直接执行 |
| RedisLocker | 多副本（在线） | Redis SET NX |
| K8sLocker | 多副本（离线） | K8s Lease 资源 |

**包间关系**：
- xdlock 可被 xcache 使用（通过 WithExternalLock）
- xcron 集成 xretry（重试）、xmetrics（追踪）、xlog（日志）
