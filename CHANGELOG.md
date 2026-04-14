# 变更日志

本文件记录 XKit 已发布版本的变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

> 当前进度与包稳定性不在此记录，见 [`docs/06-progress.md`](docs/06-progress.md)。
> 关键设计决策见 [`docs/05-decisions/`](docs/05-decisions/00-index.md)。

## [未发布]

（尚未发布首个 tag。首次发布后在此追加版本段。）

---

## 开发说明

### 集成测试

消息队列、存储相关测试需要 `integration` build tag：

```bash
go test -tags=integration ./pkg/mq/...
```

容器编排见 [`deploy/integration/README.md`](deploy/integration/README.md)。
