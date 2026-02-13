# 01 — 工具包审查

> 通用方法论见 [00-methodology.md](00-methodology.md)
> 依赖层级：**Level 1（基础层）** — 无内部包依赖，被上层广泛使用

## 审查范围

```
pkg/util/
├── xid/       # 分布式 ID 生成（Sonyflake v2），被 xsemaphore 依赖
├── xfile/     # 文件路径安全工具（路径穿越防护），被 xrotate 依赖
├── xjson/     # JSON 序列化/反序列化工具
├── xkeylock/  # Key 级别细粒度锁
├── xlru/      # LRU 缓存，被 xauth 依赖
├── xmac/      # MAC 地址工具
├── xnet/      # IP 地址操作与分类
├── xpool/     # Worker Pool 并发任务处理
├── xproc/     # 进程工具
├── xsys/      # 系统信息工具
└── xutil/     # 泛型工具函数
```

## 推荐 Skills

```
/code-reviewer   — 综合质量扫描
/go-style        — 命名规范、接口设计
/golang-patterns — 并发模式（xpool, xkeylock）、泛型模式（xutil）
/go-test         — 测试覆盖率与质量
```

---

## 模块职责与审查要点

### xid — 分布式 ID 生成（关键基础设施）

**职责**：基于 Sonyflake v2 生成分布式唯一 ID，支持多层 Machine ID 回退策略。

**重点审查**：
- [ ] **单例安全**：`Init()` 是否幂等？`sync.Once` 使用是否正确？重复调用 `Init()` 的行为是否明确？
- [ ] **溢出处理**：Sonyflake 39-bit 时间戳溢出时（约 174 年），错误是否正确传播而非 panic？
- [ ] **Machine ID 回退链**：环境变量 → 主机名哈希 → 私有 IP 哈希，每一层失败时是否有明确错误？
- [ ] **并发安全**：`New()` 在高并发下是否安全？底层 Sonyflake 的锁机制是否足够？
- [ ] **Parse/Decompose 纯度**：是否为纯函数（无副作用、无全局状态依赖）？
- [ ] **环境变量校验**：`XID_MACHINE_ID` 非法值（如 "abc"、负数、超范围）是否 fail-fast？
- [ ] **IDGeneratorFunc 接口**：是否足够通用以被 xsemaphore 等包注入？

### xfile — 文件路径安全

**职责**：路径格式校验、路径穿越防护、安全拼接。

**重点审查**：
- [ ] **路径穿越**：`../` 序列、符号链接、编码绕过（`%2e%2e`）是否全部防护？
- [ ] **SanitizePath**：对 Windows 路径（`C:\`、`\\`）是否有处理（即使目标平台是 Linux）？
- [ ] **SafeJoin**：`filepath.Clean` + 前缀检查是否存在 TOCTOU race？
- [ ] **符号链接**：`SafeJoinWithOptions` 的 symlink 解析是否限制递归深度？
- [ ] **EnsureDir**：权限模式是否合理（不应创建 0777 目录）？

### xkeylock — Key 级别细粒度锁

**重点审查**：
- [ ] **内存泄漏**：无人持有的 key 对应的锁是否被回收？
- [ ] **死锁**：嵌套加锁（lock key A → lock key B → lock key A）是否有防护？
- [ ] **公平性**：同一 key 的等待者是否 FIFO？饥饿问题？
- [ ] **性能**：高并发不同 key 时，全局锁（如 sync.Map 的内部锁）是否成为瓶颈？

### xlru — LRU 缓存

**重点审查**：
- [ ] **并发安全**：读写是否用锁保护？锁粒度是否合理？
- [ ] **淘汰正确性**：LRU 淘汰顺序是否正确？Get 操作是否正确提升优先级？
- [ ] **容量边界**：容量为 0、1、负数时的行为是否合理？
- [ ] **内存效率**：节点是否有不必要的内存占用？大量小 key 场景下 overhead 如何？

### xpool — Worker Pool

**重点审查**：
- [ ] **goroutine 生命周期**：提交任务后 pool 关闭，在途任务是否等待完成？
- [ ] **panic 恢复**：worker goroutine 中的 panic 是否 recover 并记录，不影响其他 worker？
- [ ] **背压控制**：任务队列满时的行为（阻塞/拒绝/丢弃）是否可配置？
- [ ] **context 支持**：是否支持 context cancellation 取消排队中的任务？
- [ ] **资源回收**：Close 后是否等待所有 goroutine 退出？是否有 goroutine 泄漏检测？

### xnet — IP 地址操作

**重点审查**：
- [ ] **IPv6 支持**：所有函数是否同时支持 IPv4 和 IPv6？
- [ ] **私有 IP 判断**：是否覆盖 RFC 1918（10.0/172.16/192.168）、RFC 4193（fd00::/8）、链路本地等？
- [ ] **边界输入**：空字符串、非法 IP、IPv4-mapped IPv6（`::ffff:192.168.1.1`）是否正确处理？

### xutil — 泛型工具

**重点审查**：
- [ ] **泛型约束**：类型约束是否够精确（`comparable` vs `any` vs 自定义约束）？
- [ ] **标准库重复**：是否与 `slices`、`maps`、`cmp` 等标准库包重复？Go 1.25 新增了哪些可替代的？
- [ ] **nil slice/map 安全**：输入为 nil 时是否有合理行为而非 panic？

### xjson、xmac、xproc、xsys — 轻量工具

**重点审查**：
- [ ] 是否有足够理由独立成包（如只有 2-3 个函数，是否应合并）？
- [ ] 函数签名是否与标准库风格一致（返回 `(T, error)` 而非 panic）？
- [ ] 是否存在平台相关代码未用 build tag 隔离？

---

## 跨包一致性检查

- [ ] **错误变量命名**：所有包的 `ErrXxx` 是否遵循统一前缀和风格？
- [ ] **Options 模式**：使用 Functional Options 的包是否统一命名为 `WithXxx`，Option 类型是否统一？
- [ ] **文档完整性**：每个包是否有 `doc.go`？导出符号是否有中文注释？
- [ ] **Example 测试**：公开 API 是否有 `Example_xxx` 可运行示例？
- [ ] **不存在跨包工具调用**：util 包之间不应互相 import（保持各自独立）
