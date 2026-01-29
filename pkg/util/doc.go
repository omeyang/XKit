// Package util 提供通用工具相关的子包。
//
// 子包列表：
//   - xfile: 文件操作工具，目录创建、路径处理等
//   - xjson: JSON 序列化工具，Pretty 格式化输出
//   - xkeylock: 基于 key 的进程内互斥锁，支持 context 超时和非阻塞获取
//   - xlru: LRU 缓存，泛型支持、自动 TTL 过期
//   - xmac: MAC 地址工具库，多格式解析、验证、序列化
//   - xnet: IP 地址工具库，基于 net/netip + go4.org/netipx 的增量函数（格式化、解析、序列化兼容）
//   - xpool: 泛型 Worker Pool，可配置 worker/队列大小、优雅关闭
//   - xproc: 进程信息查询，PID 和进程名称
//   - xsys: 系统资源限制管理，文件描述符上限
//   - xutil: 泛型工具函数，三目运算符
//
// 设计原则：
//   - 提供常用的文件和路径操作封装
//   - 安全处理路径遍历和符号链接
//   - 跨平台兼容
package util
