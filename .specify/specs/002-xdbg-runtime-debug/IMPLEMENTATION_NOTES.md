# xdbg 实现注记

本文档记录 xdbg 模块的关键设计决策和实现细节，供后续维护参考。

## 1. Transport 可注入性设计

### 背景

`Server` 支持通过 `WithTransport()` 注入自定义传输层，主要用于：
- 单元测试中使用 mock Transport
- 特殊场景下的自定义传输实现

### 设计要点

```go
type Server struct {
    transport       Transport
    customTransport bool  // 标记是否使用了用户自定义的 Transport
}
```

**关键行为**：

1. **Start() 阶段**：如果 `opts.Transport != nil`，使用用户提供的 Transport 并设置 `customTransport = true`

2. **stopListening() 阶段**：
   - 非自定义 Transport：Close 后创建新的 `UnixTransport` 实例
   - 自定义 Transport：仅 Close，不创建新实例

**原因**：自定义 Transport 的生命周期由用户管理。如果自动替换为 `UnixTransport`，会导致测试中的 mock 失效。

### 测试注意事项

如果 mock Transport 需要支持 Disable/Enable 循环，mock 实现应支持：
- `Close()` 后可重新调用 `Listen()`
- 或者测试用例在每次 Enable 前重新配置 Transport

## 2. CLI 快捷命令机制

### 背景

规格文档 (spec.md FR-33) 规定支持单命令模式：

```bash
xdbgctl setlog debug    # 而不是 xdbgctl exec setlog debug
```

### 实现方式

通过 `createShortcutCommand()` 工厂函数创建快捷命令：

```go
createShortcutCommand("setlog", "查看/设置日志级别", "[level]")
```

快捷命令内部调用 `cmdExec()`，将命令名作为第一个参数：

```go
args := append([]string{name}, cmd.Args().Slice()...)
return cmdExec(ctx, socketPath, timeout, args)
```

### 支持的快捷命令

| 快捷命令 | 等价于 |
|---------|--------|
| `xdbgctl setlog [level]` | `xdbgctl exec setlog [level]` |
| `xdbgctl stack` | `xdbgctl exec stack` |
| `xdbgctl freemem` | `xdbgctl exec freemem` |
| `xdbgctl pprof <sub>` | `xdbgctl exec pprof <sub>` |
| `xdbgctl breaker [name]` | `xdbgctl exec breaker [name]` |
| `xdbgctl limit [name]` | `xdbgctl exec limit [name]` |
| `xdbgctl cache [name]` | `xdbgctl exec cache [name]` |
| `xdbgctl config` | `xdbgctl exec config` |

`exec` 子命令仍然保留，用于执行任意调试命令。

## 3. Socket 清理安全策略

### 背景

`UnixTransport.Listen()` 在启动时需要清理可能残留的 socket 文件。原实现使用 `os.Remove()` 直接删除，存在安全风险：如果路径被误配置为普通文件，会导致数据丢失。

### 安全策略

```go
info, err := os.Stat(t.socketPath)
if err == nil {
    if info.Mode()&os.ModeSocket == 0 {
        return fmt.Errorf("path exists but is not a socket: %s", t.socketPath)
    }
    // 是 socket，安全删除
    os.Remove(t.socketPath)
}
```

**行为**：
- 路径不存在：正常继续
- 路径是 socket 文件：删除后继续
- 路径是其他类型文件：返回错误，拒绝覆盖

## 4. PID 发现策略

### 背景

`xdbgctl enable` 需要向目标进程发送 SIGUSR1 信号。在容器环境中，自动发现进程 PID 可能失败。

### 策略演进

**原策略**（有风险）：
- 发现失败时自动回退到 PID 1（容器主进程）
- 风险：多进程容器可能误伤

**当前策略**：
- 发现失败时不自动回退
- 要求用户通过 `--pid` 明确指定目标进程
- 在容器环境中给出更友好的提示

```
无法自动发现进程: <error>
在容器环境中，请使用 --pid 1（如主进程是目标）或指定具体 PID
```

### 发现机制

1. 获取 socket 文件的 inode
2. 扫描 `/proc/*/fd/` 查找持有该 inode 的进程
3. 返回第一个匹配的 PID

**限制**：
- 需要读取 `/proc` 的权限
- 某些容器配置可能限制 `/proc` 访问

## 5. 输出截断与编码

### 背景

命令输出在 JSON 编码前通过 `TruncateOutput()` 截断。但转义字符（如 `\n`、`\"`）会导致编码后长度增加。

### 当前处理

1. `MaxOutputSize` 在编码前截断
2. 如果编码后仍超限，由 `sendEncodingErrorResponse` 返回错误响应

### 可选改进（未实现）

先尝试编码，如果失败则逐步减少输出大小重试。

当前降级策略已足够，编码失败是极端情况。

---

*最后更新: 2026-01-23*
