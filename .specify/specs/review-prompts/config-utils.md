# Config & Utils 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/config/
└── xconf/        # 配置加载与热重载，基于 koanf

pkg/util/
├── xfile/        # 文件路径安全工具
└── xpool/        # Worker Pool

internal/
└── （相关内部实现）
```

## 模块职责

**包职责**：
- **xconf**：
  - 配置加载（YAML/JSON 格式自动检测）
  - 热重载（Watch 监视文件变化，内置防抖）
  - Client() 暴露底层 koanf

- **xfile**：
  - SanitizePath：路径格式检查，防止穿越
  - SafeJoin：确保结果在 base 目录内
  - SafeJoinWithOptions：支持符号链接解析
  - EnsureDir：确保文件的父目录存在

- **xpool**：
  - Worker Pool 并发任务处理

**包间关系**：
- xrotate 使用 xfile.SanitizePath 和 EnsureDir
- xlog 集成 xrotate
