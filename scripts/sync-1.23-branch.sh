#!/usr/bin/env bash
# XKit develop-1.23-release 分支自动同步探测脚本
#
# 职责：
#   1. fetch origin
#   2. 探测 origin/main 是否有新 commit 未被 develop-1.23-release 吸收
#   3. 若有，在本地 auto/sync-1.23-<date> 分支上执行"同步范式"：
#      merge main → 重新应用降级 → go mod tidy+回退版本上限 → task lint/test/build
#   4. **不自动 push**。结果留在本地分支，日志写 docs/sync-1.23-log.md 等待人工 review
#
# 触发：cron（见 crontab）
# 用法：sync-1.23-branch.sh
set -euo pipefail

REPO=/root/code/go/src/github.com/omeyang/XKit

# cron 非交互 shell 不加载 ~/.zshrc，显式 PATH + IS_SANDBOX=1 放行 root + --dangerously-skip-permissions
export PATH="/root/.local/share/fnm/node-versions/v24.14.1/installation/bin:/usr/local/go/bin:/usr/local/bin:/root/.local/bin:/root/code/go/bin:/usr/bin:/bin"
export HOME=/root
export IS_SANDBOX=1

LOG_DIR="$REPO/.sync-1.23-runs"
mkdir -p "$LOG_DIR"
TS=$(date -u +%Y%m%dT%H%M%SZ)
DATE_CST=$(TZ=Asia/Shanghai date +%Y-%m-%d)
RUNLOG="$LOG_DIR/sync-${TS}.log"
exec > >(tee -a "$RUNLOG") 2>&1

LOG_FILE="$REPO/docs/sync-1.23-log.md"

append_log() {
  local status="$1" detail="$2"
  # 先创建带 header 的空文件（若不存在），再 append 条目——否则 {} >>FILE 结构会先
  # 创建文件再做 [[ -f ]] 检查，header 永远写不进去
  [[ -f "$LOG_FILE" ]] || printf '# 1.23 Branch Sync Log\n' > "$LOG_FILE"
  {
    echo ""
    echo "## $DATE_CST"
    echo "- 状态: $status"
    echo "- 详情: $detail"
    echo "- 运行日志: $RUNLOG"
  } >> "$LOG_FILE" 2>/dev/null || true
}

# 依赖体检
for bin in claude task git go awk; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "FATAL: dependency '$bin' not found in PATH=$PATH"
    append_log "FAILED (依赖缺失)" "$bin 不在 PATH"
    exit 2
  fi
done

cd "$REPO"

# 并发锁：防止本脚本与自身或与对抗审查并发改工作区
exec 200>"$LOG_DIR/.lock"
if ! flock -n 200; then
  echo "another sync in progress; skip this run"
  append_log "SKIP (并发锁)" "另一个 sync 进程在跑，跳过"
  exit 0
fi

echo "=== [$(date -Iseconds)] sync start ==="

# 工作区必须干净，避免撞上人工进行中的修改
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "FATAL: working tree not clean"
  append_log "ABORT (工作区脏)" "检测到未提交改动，跳过自动同步"
  exit 1
fi

# 记下当前分支，结束时恢复
ORIG_BRANCH=$(git rev-parse --abbrev-ref HEAD)

restore_branch() {
  git checkout "$ORIG_BRANCH" --quiet 2>/dev/null || true
}
trap restore_branch EXIT

git fetch origin main develop-1.23-release --quiet

# 探测：main 是否有新 commit 未在 develop-1.23-release
NEW_COMMITS=$(git log --oneline origin/develop-1.23-release..origin/main 2>/dev/null || true)
if [[ -z "$NEW_COMMITS" ]]; then
  echo "no new commits on main; branches in sync"
  append_log "OK (无差异)" "origin/main 无新 commit 需同步"
  exit 0
fi

echo "new commits on main to sync:"
echo "$NEW_COMMITS"
echo "---"

# 创建独立同步分支，不碰 develop-1.23-release 本体
SYNC_BRANCH="auto/sync-1.23-${DATE_CST}"
# 若今日分支已存在（脚本重试），覆盖重来
git branch -D "$SYNC_BRANCH" 2>/dev/null || true
git checkout -b "$SYNC_BRANCH" origin/develop-1.23-release --quiet

# 备份 tag，便于回滚或人工审查时对比
BACKUP_TAG="backup/develop-1.23-release-pre-sync-${DATE_CST}"
git tag -f "$BACKUP_TAG" origin/develop-1.23-release

echo "working on branch: $SYNC_BRANCH (backup tag: $BACKUP_TAG)"

# 调 Claude Code 执行"同步范式"——因降级涉及语法替换、版本天花板维护、
# 冲突解决等非平凡判断，需要 LLM 参与。脚本不自己做这些。
CLAUDE_PROMPT=$(cat <<EOF
你是 XKit develop-1.23-release 分支自动同步器（Go 1.23 兼容分支）。

## 上下文
- 仓库：$REPO
- 当前所在分支：$SYNC_BRANCH（基于 origin/develop-1.23-release，已备份 tag $BACKUP_TAG）
- 目标：把 origin/main 的新 commit 合进本分支，重新应用 Go 1.23 降级，跑过 lint/test/build
- **绝对禁止 push 远端**（任何形式的 git push 都不允许）

## origin/main 新 commit（待同步）
\`\`\`
$NEW_COMMITS
\`\`\`

## 必须严格按以下流程执行

### 步骤 1：merge main（冲突统一取 main 侧）
\`\`\`
git merge origin/main --no-edit
\`\`\`
若有冲突：对每个冲突文件 \`git checkout --theirs <file>\`，再 \`git add <file>\`，最后 \`git commit --no-edit\`。

### 步骤 2：重新应用 Go 1.23 降级
**逐项检查并修复**（发现不符合的地方用 Edit 工具修）：

1. \`go.mod\` 顶部必须是 \`go 1.23.0\`，不能有 \`toolchain\` 指令
2. \`.github/workflows/ci.yml\` 中 \`GO_VERSION: '1.23.0'\`
3. \`Taskfile.yml\` 的 verify task 中 grep 字符串为 \`"go1.23"\`
4. \`README.md\`、\`docs/03-conventions/02-contributing.md\` 中的 Go 版本字符串应为 1.23
5. **全仓库**搜 \`wg\\.Go(\`（任意 WaitGroup 变量的 \`.Go\` 调用）：
   替换为 \`wg.Add(1); go func() { defer wg.Done(); <原闭包体> }()\`
6. **全仓库**搜 \`for b\\.Loop()\`：
   替换为 \`for i := 0; i < b.N; i++\`
7. \`pkg/distributed/xdlock/etcd.go\` 的 \`wrapEtcdError\` 中 **不能**出现 \`concurrency.ErrLockReleased\` 分支（etcd v3.5.21 未导出）；如果 merge 时被 \`--theirs\` 带进来，删掉并加回下面的注释：
   \`\`\`
   // 1.23 分支行为差异: 本分支 etcd 客户端锁定在 v3.5.21，未导出 concurrency.ErrLockReleased
   // (v3.6+ 新增)，因此"锁被 session 提前释放"的错误不会映射为 ErrNotLocked，调用方无法
   // 通过 errors.Is(err, ErrNotLocked) 判定这一场景。详见 docs/1.23-branch-notes.md。
   \`\`\`
8. \`pkg/mq/xpulsar/auth_test.go\` 的 "valid params" 子测试**不应**断言 \`method.auth != nil\`（v0.16 构造时校验可能产生 nil auth）
9. \`docs/1.23-branch-notes.md\` 必须保留（如果 merge 产生冲突或丢失，从 $BACKUP_TAG 恢复）

### 步骤 3：依赖版本上限（memory/project_1_23_branch.md 记录）
执行：
\`\`\`
go mod tidy
go mod edit -go=1.23.0
go mod tidy
\`\`\`
然后逐个回退到 1.23 兼容版本（使用 \`go get <pkg>@<version>\` 再 \`go mod tidy\`）：
- google.golang.org/grpc → v1.71.0
- go.opentelemetry.io/otel* → v1.38.0
- golang.org/x/crypto → v0.41.0, x/net → v0.43.0, x/sys → v0.35.0, x/sync → v0.16.0, x/text → v0.28.0, x/term → v0.34.0, x/mod → v0.27.0
- go.etcd.io/etcd/* → v3.5.21
- go-jose/v4 → v4.0.5, oauth2 → v0.28.0, auto/sdk → v1.2.0

直接 grep \`go.mod\` 检查是否有版本超标的直接依赖；若有 indirect 被顶到 1.24，用 \`go get ...@\` 回退到能容纳的最新 1.23 兼容版。

### 步骤 4：commit 降级改动
若步骤 2/3 产生了改动：
\`\`\`
git add -A
git commit -m "refactor: 重新应用 Go 1.23 降级（同步 main 后）"
\`\`\`
**禁 Co-Authored-By / Claude 署名**（见 memory/feedback_no_coauthor.md）。

### 步骤 5：验证
执行：
\`\`\`
task lint
task test
task build
\`\`\`
**不跑 \`task vulncheck\`**（本分支长期有 2 个不可修复 CVE，会失败，属预期）。

任何一步失败：
- 读日志，分析根因
- 尝试修复（最多 3 轮）
- 仍败则执行 \`git reset --hard $BACKUP_TAG\`，保留 $SYNC_BRANCH 指向此处，并明确报告失败阶段

### 步骤 6：**不要 push**。结束时结构化汇报：
- \`SYNC_STATUS\`: OK / FAILED
- \`MERGE_COMMIT\`: <hash>
- \`DOWNGRADE_COMMIT\`: <hash 或 "无">
- \`TASK_LINT\`: pass / fail
- \`TASK_TEST\`: pass / fail
- \`TASK_BUILD\`: pass / fail
- \`NOTES\`: 任何需要人工注意的事（新引入的非平凡降级、依赖冲突、潜在行为差异）

## 硬约束
- **严禁** git push / git push --force / git push --no-verify（任何形式）
- **严禁** 修改 crontab 或 .github/workflows/ 以外的 CI 配置
- **严禁** \`_ = expr\` 这种 errcheck 绕过（见 MEMORY.md .golangci.yml check-blank:true）
- 中文注释英文标识符
- 构造器返 error 不 panic
- 若意外失败立即停止，报告原因，不要绕过
EOF
)

set +e
claude -p "$CLAUDE_PROMPT" \
  --dangerously-skip-permissions \
  --model claude-opus-4-6
CLAUDE_RC=$?
set -e

# 结束时必定不在 push 状态（脚本没调 push），但验证一下
CURRENT_HEAD=$(git rev-parse HEAD)
CURRENT_BRANCH_FINAL=$(git rev-parse --abbrev-ref HEAD)

if [[ $CLAUDE_RC -ne 0 ]]; then
  append_log "FAILED (Claude rc=$CLAUDE_RC)" \
    "分支 $SYNC_BRANCH HEAD=$CURRENT_HEAD 残留本地；备份 tag $BACKUP_TAG；详见运行日志"
  echo "=== sync failed rc=$CLAUDE_RC ==="
  exit $CLAUDE_RC
fi

# 成功路径：本地分支已就绪，等待人工 review + push
append_log "OK (本地准备完成，未推送)" \
  "分支 $SYNC_BRANCH HEAD=$CURRENT_HEAD；备份 tag $BACKUP_TAG；新 main commit 数：$(echo "$NEW_COMMITS" | wc -l)"

echo ""
echo "=== sync prepared (NOT pushed) ==="
echo "  branch:     $SYNC_BRANCH"
echo "  head:       $CURRENT_HEAD"
echo "  backup tag: $BACKUP_TAG"
echo "  runlog:     $RUNLOG"
echo ""
echo "人工 review 后，若满意："
echo "  git checkout develop-1.23-release"
echo "  git merge --ff-only $SYNC_BRANCH"
echo "  git push origin develop-1.23-release --no-verify"
echo ""
echo "若放弃："
echo "  git branch -D $SYNC_BRANCH"
echo "  git tag -d $BACKUP_TAG"
