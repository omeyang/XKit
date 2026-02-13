// Package xid 提供分布式唯一 ID 生成能力，基于 Sony Sonyflake 算法实现。
//
// # 设计理念
//
// xid 是对 sony/sonyflake 的薄封装，提供项目内统一的 ID 生成入口。
// 主要特点：
//   - 单例模式，全局共享一个生成器实例
//   - 智能机器 ID 获取策略，支持离线 K8s 等多种环境
//   - 生成的 ID 具有时序性，便于调试和排查
//   - 比 UUID 更高效（~50ns vs ~500ns）且更短（13字符 vs 36字符）
//
// # ID 结构
//
// Sonyflake ID 由以下部分组成（默认配置）：
//
//	39 bits - 时间戳（10ms 为单位，可用约 174 年）
//	 8 bits - 序列号（同一时间单位内最多 256 个 ID）
//	16 bits - 机器 ID（最多 65536 台机器）
//
// # 快速开始
//
// 基本用法（推荐）：
//
//	// 生成 ID（字符串格式，base36 编码，推荐用法）
//	id := xid.MustNewStringWithRetry()  // 例如: "1a2b3c4d5e6f7"
//
//	// 生成 ID（int64 格式）
//	id, err := xid.New()
//	if err != nil {
//	    return err
//	}
//
// 带错误处理：
//
//	id, err := xid.NewString()
//	if err != nil {
//	    // 处理错误（通常是时钟回拨）
//	    return err
//	}
//
// # 自定义配置
//
// 如果需要自定义机器 ID 或其他配置，可以在应用启动时调用 Init：
//
//	func main() {
//	    if err := xid.Init(
//	        xid.WithMachineID(func() (uint16, error) {
//	            // 自定义机器 ID 生成逻辑
//	            return getMyMachineID()
//	        }),
//	    ); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // 应用代码...
//	}
//
// # 机器 ID 获取策略
//
// xid 使用多层回退策略获取机器 ID，确保在各种环境下都能正常工作：
//
//  1. XID_MACHINE_ID 环境变量（直接指定数字 0-65535）
//  2. POD_NAME 环境变量的哈希值（K8s Downward API）
//  3. HOSTNAME 环境变量的哈希值
//  4. os.Hostname() 的哈希值
//  5. 私有 IP 地址的低 16 位
//
// 这种策略适用于：
//   - 在线/离线 K8s 集群
//   - HostNetwork 模式
//   - 虚拟机、物理机、容器
//
// # 机器 ID 碰撞风险
//
// 使用哈希方式获取机器 ID 时（策略 2-4），存在碰撞风险。
// 根据生日悖论计算，在 65536（2^16）的空间内：
//
//   - 10 节点：约 0.08% 碰撞概率
//   - 50 节点：约 1.9% 碰撞概率
//   - 100 节点：约 7.3% 碰撞概率
//   - 200 节点：约 26% 碰撞概率
//
// 适用规模建议：
//
//   - 小规模（<50 节点）：直接使用默认策略，碰撞概率可接受
//   - 中等规模（50-200 节点）：建议配置 POD_NAME 环境变量
//   - 大规模（>200 节点）：强烈建议通过 XID_MACHINE_ID 显式分配唯一 ID
//
// 如果发生碰撞，两台机器在同一 10ms 时间窗口内生成的 ID 可能重复。
// 对于高一致性要求的场景，请使用 XID_MACHINE_ID 环境变量显式分配。
//
// # K8s 环境配置
//
// ## 推荐配置（Downward API）
//
// 在 Deployment 中注入 POD_NAME，自动获取唯一的机器 ID：
//
//	spec:
//	  containers:
//	  - name: app
//	    env:
//	    - name: POD_NAME
//	      valueFrom:
//	        fieldRef:
//	          fieldPath: metadata.name
//
// ## 离线 K8s 集群
//
// 离线集群无法访问外部网络，但 xid 不依赖任何外部服务：
//   - 不需要云厂商元数据服务
//   - 不需要外部 NTP（但建议配置集群内 NTP）
//   - Pod IP 由 CNI 内部分配，离线环境正常工作
//
// ## HostNetwork 模式
//
// 使用 HostNetwork 时，多个 Pod 共享宿主机 IP。
// 必须通过 POD_NAME 或 XID_MACHINE_ID 环境变量区分：
//
//	spec:
//	  hostNetwork: true
//	  containers:
//	  - name: app
//	    env:
//	    - name: POD_NAME
//	      valueFrom:
//	        fieldRef:
//	          fieldPath: metadata.name
//
// ## 显式指定机器 ID
//
// 如需严格控制机器 ID（避免哈希碰撞），可直接指定：
//
//	spec:
//	  containers:
//	  - name: app
//	    env:
//	    - name: XID_MACHINE_ID
//	      value: "12345"
//
// # 时钟回拨处理
//
// Sonyflake 在检测到时钟回拨时会返回错误。xid 包提供了自动重试机制：
//
// ## 推荐用法（生产环境）
//
// 使用 WithRetry 后缀的方法，自动处理短暂的时钟回拨：
//
//	id := xid.MustNewStringWithRetry()  // 推荐
//
// 这些方法在检测到时钟回拨时会自动等待并重试（默认最多 500ms），
// 能够容忍 NTP 同步等场景导致的短暂时钟回拨。
//
// ## 配置等待参数
//
// 可以在初始化时配置等待时间：
//
//	xid.Init(
//	    xid.WithMaxWaitDuration(1 * time.Second),  // 最大等待 1 秒
//	    xid.WithRetryInterval(5 * time.Millisecond), // 每 5ms 重试一次
//	)
//
// ## 不带重试的方法
//
// 如果不希望等待，可以使用不带 WithRetry 后缀的方法：
//
//	id, err := xid.NewString()  // 时钟回拨时立即返回错误
//	id := xid.MustNewString()   // 时钟回拨时 panic
//
// ## 最佳实践
//
//   - 生产环境确保 NTP 配置正确
//   - 使用 MustNewStringWithRetry 作为默认选择
//   - 监控 ErrClockBackwardTimeout 错误，及时发现时钟问题
//
// # 线程安全
//
// xid 包的所有公开函数都是线程安全的，可以被多个 goroutine 并发调用。
//
// # 与 UUID 对比
//
//	| 特性       | xid (Sonyflake)     | UUID v4            |
//	|------------|---------------------|--------------------|
//	| 生成速度   | ~50ns               | ~500ns             |
//	| 字符串长度 | 13 字符 (base36)    | 36 字符            |
//	| 时序性     | 有（可排序）        | 无                 |
//	| 唯一性保证 | 时间+机器+序列      | 随机数             |
//	| 配置需求   | 可选                | 无                 |
package xid
