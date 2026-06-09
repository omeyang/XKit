---
package: pkg/security/xtls
stability: Beta
coverage: 90.5%
tags: [security, tls, mtls, grpc]
---

# pkg/security/xtls

TLS 配置加载与构建。文件路径或 inline 字节均可；支持 mTLS；提供 gRPC `credentials.TransportCredentials` 工厂。

## 用途

- HTTP/gRPC 服务端启用 TLS / mTLS
- gRPC 客户端使用证书
- 测试场景用 inline 证书快速搭起

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/security/xtls"

// 服务端
cfg, _ := xtls.BuildServerTLSConfig(xtls.Config{
    CertFile: "/etc/certs/server.crt",
    KeyFile:  "/etc/certs/server.key",
    CAFile:   "/etc/certs/ca.crt",
    Require:  true,  // mTLS
})
listener, _ := tls.Listen("tcp", ":443", cfg)

// gRPC 凭据
creds, _ := xtls.ServerCredentials(cfg)
server := grpc.NewServer(grpc.Creds(creds))
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Config` | 证书/密钥/CA（文件路径或 inline 字节）|
| `BuildServerTLSConfig(c) (*tls.Config, error)` | 服务端 |
| `BuildClientTLSConfig(c) (*tls.Config, error)` | 客户端 |
| `ServerCredentials / ClientCredentials` | gRPC 凭据 |

## 设计要点

- **MinVersion = TLS1.2**：默认强制
- **Inline 优先于文件**：inline 字节存在时忽略文件路径
- **`Require=false` 单向 TLS**：CA 忽略
- **空 CA 文件**：拒绝（避免误以为 mTLS 实际无 trust）

## 相关

- API：[api.md#pkgsecurityxtls](../../03-conventions/01-api.md#pkgsecurityxtls)
