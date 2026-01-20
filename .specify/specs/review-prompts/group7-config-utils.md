# 模块审查：配置与工具（xconf, xfile, xrotate）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/config/
└── xconf/        # 配置加载与热重载，基于 koanf

pkg/util/
└── xfile/        # 文件路径安全工具

pkg/observability/
└── xrotate/      # 日志文件轮转，基于 lumberjack v2
```

设计理念：
- xconf：与 xcache/xmq 一致的"工厂函数 + Client() 暴露"模式
- xfile：纯工具函数，无状态，专注路径安全
- xrotate：Rotator 接口抽象，基于 lumberjack 实现

包间关系：
```
xfile (路径安全工具)
    ↓ 被使用
xrotate (日志轮转) → 使用 xfile.SanitizePath 和 EnsureDir
    ↓ 被使用
xlog (日志系统) → 集成 xrotate
```

---

## xconf：配置加载与热重载

**职责**：基于 koanf 提供配置加载、热重载和文件监视功能。

**关键文件**：
- `config.go` - Config 类型和加载逻辑
- `watch.go` - Watcher 文件监视器
- `compat.go` - gobase/xconf 兼容性函数

**核心接口**：
```go
type Config interface {
    Client() *koanf.Koanf           // 暴露底层 koanf
    Reload() error                  // 并发安全的重载
    Unmarshal(path string, v any) error
    Path() string                   // 配置文件路径（从 bytes 创建时为空）
}
```

**创建方式**：
```go
// 从文件加载（格式根据扩展名自动检测）
cfg, err := xconf.New("/etc/app/config.yaml")

// 从字节数据加载（适用于 K8s ConfigMap）
cfg, err := xconf.NewFromBytes(data, xconf.FormatYAML)

// 兼容性函数（与 gobase/xconf 兼容）
var config MyConfig
err := xconf.Load("/etc/app/config.yaml", &config)
```

**文件监视**：
```go
w, err := xconf.Watch(cfg, func(c xconf.Config, err error) {
    if err != nil {
        log.Printf("reload failed: %v", err)
        return
    }
    log.Println("config reloaded successfully")
})
defer w.Stop()
w.StartAsync()
```

监视器特性：
- 监视目录而非文件（正确处理编辑器保存行为）
- 内置防抖（默认 100ms），可通过 WithDebounce 配置
- 从 bytes 创建的 Config 不支持监视

**并发安全警告**：
```go
// 正确：每次需要配置时调用 Client()
value := cfg.Client().String("key")

// 错误：缓存 Client() 引用可能导致读取过期数据
k := cfg.Client()
// ... Reload() 被调用 ...
value := k.String("key") // 可能读取到旧值
```

---

## xfile：文件路径安全工具

**职责**：提供安全的路径操作函数，防止路径穿越攻击。

**关键文件**：
- `path.go` - 路径安全函数
- `dir.go` - 目录操作函数

**核心函数**：
| 函数 | 功能 | 使用场景 |
|-----|------|---------|
| SanitizePath | 检查路径格式，防止穿越 | 通用路径验证 |
| SafeJoin | 确保结果在 base 目录内 | 用户输入处理 |
| SafeJoinWithOptions | 增强版，支持符号链接解析 | 高安全场景 |
| EnsureDir | 确保文件的父目录存在 | 文件创建前准备 |

**路径穿越检测**：
使用精确的路径段匹配，只有 ".." 作为独立路径段时才被视为穿越攻击：

```go
SafeJoin("/var/log", "..config")      // ✓ 合法 -> "/var/log/..config"
SafeJoin("/var/log", "../etc/passwd") // ✗ 拒绝 -> 路径穿越
SafeJoin("/var/log", "app..2024.log") // ✓ 合法
```

**符号链接安全**：
```go
// 默认不解析符号链接（大多数场景适用）
SafeJoin("/var/log", "app.log")

// 高安全场景（用户上传、沙箱目录）
SafeJoinWithOptions(base, path, SafeJoinOptions{ResolveSymlinks: true})
```

符号链接风险示例：
```
# 攻击者在 /var/log 内创建了符号链接
/var/log/evil -> /etc

SafeJoin("/var/log", "evil/passwd")
// 返回 "/var/log/evil/passwd"，但实际指向 /etc/passwd
```

**目录操作**：
```go
// 确保文件的父目录存在（默认权限 0750）
err := xfile.EnsureDir("/var/log/app/app.log")

// 使用指定权限
err := xfile.EnsureDirWithPerm("/var/log/app/app.log", 0755)
```

---

## xrotate：日志文件轮转

**职责**：提供日志文件轮转功能，基于 lumberjack v2 实现。

**关键文件**：
- `rotator.go` - Rotator 接口定义
- `lumberjack.go` - lumberjack 实现

**核心接口**：
```go
type Rotator interface {
    Write(p []byte) (n int, err error)  // 并发安全
    Close() error
    Rotate() error                      // 手动轮转
}
```

**创建轮转器**：
```go
rotator, err := xrotate.NewLumberjack("/var/log/app.log",
    xrotate.WithMaxSize(500),      // MB，默认 500
    xrotate.WithMaxBackups(7),     // 备份数量，默认 7
    xrotate.WithMaxAge(30),        // 保留天数，默认 30
    xrotate.WithCompress(true),    // 压缩备份，默认 true
    xrotate.WithFileMode(0644),    // 文件权限，默认 0600
)
```

**文件权限处理**：
- lumberjack v2.2+ 默认使用 0600 权限创建文件
- WithFileMode 通过写入后 chmod 调整权限
- 存在短暂时间窗口文件权限为 0600（通常可忽略）

权限应用场景：
- 首次写入创建文件后
- lumberjack 自动轮转创建新文件后
- 外部程序修改文件权限后的下一次写入

**与 xlog 集成**：
```go
logger := xlog.NewBuilder().
    WithRotation("/var/log/app.log",
        xrotate.WithMaxSize(100),
        xrotate.WithMaxBackups(10),
    ).
    Build()
```

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**xconf 并发安全**：
- Reload() 是否使用互斥锁保护？
- Client() 返回底层指针的警告是否在文档中清晰说明？
- 高并发场景下是否建议使用 RWMutex 包装？
- koanf 实例替换时是否有读取竞态？

**xconf 文件监视**：
- 是否监视目录而非文件？（处理编辑器保存行为）
- 防抖机制是否正确合并多个文件事件？
- 编辑器保存触发的 WRITE -> CHMOD -> RENAME -> CREATE 事件链如何处理？
- StartAsync 和 Stop 的并发安全性？
- 从 bytes 创建的 Config 调用 Watch 时的行为？

**xconf 配置重载**：
- 重载失败时是否保留旧配置？
- 回调函数是否正确传递错误信息？
- 格式检测（YAML/JSON）是否根据扩展名正确工作？

**xfile 路径穿越检测**：
- hasDotDotSegment 是否按路径段精确判断 `..`？
- 是否同时处理 `/` 和 `\` 分隔符？（跨平台）
- `..config` 等合法文件名是否被误判？
- 尾部斜杠是否正确识别为目录？

**xfile 符号链接处理**：
- ResolveSymlinks 是否使用 filepath.EvalSymlinks？
- 部分存在的路径如何处理？（evalSymlinksPartial）
- 解析后是否重新验证路径仍在 base 内？
- base 目录不存在时的行为？

**xrotate 并发安全**：
- Write 是否继承 lumberjack 的内部锁？
- chmod 操作是否有独立的锁保护？
- Write 和 Rotate 同时调用的行为？

**xrotate 文件权限**：
- 权限比较是否只比较权限位？（去除文件类型位）
- chmod 失败是否静默处理？（写入已成功）
- 自动轮转创建新文件后权限是否正确应用？

**xrotate 配置验证**：
- 零值是否正确替换为默认值？
- 负值是否被正确拒绝？
- 路径是否通过 xfile.SanitizePath 验证？

---

## 资源生命周期

**xconf.Config**：
```
New(path) / NewFromBytes(data, format)
    ↓
创建 koanf 实例，加载配置
    ↓
Client() → 返回底层 koanf（读取）
Reload() → 重新加载配置（使用互斥锁）
Unmarshal() → 类型安全反序列化
    ↓
（无需显式 Close，但 Watch 需要 Stop）
```

**xconf.Watcher**：
```
Watch(cfg, callback)
    ↓
创建 fsnotify.Watcher
    ↓
StartAsync() / Start() → 启动监视
    ↓
文件变更 → 防抖 → Reload → 回调
    ↓
Stop() → 停止监视，释放资源
```

**xrotate.Rotator**：
```
NewLumberjack(filename, opts...)
    ↓
xfile.SanitizePath() → 路径安全检查
xfile.EnsureDir() → 创建父目录
    ↓
创建 lumberjack.Logger
    ↓
Write() → 写入日志（可能触发自动轮转）
Rotate() → 手动轮转
    ↓
Close() → 关闭轮转器
```

