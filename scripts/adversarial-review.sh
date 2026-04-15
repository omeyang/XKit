#!/usr/bin/env bash
# XKit 自动化对抗审查 v2：Claude×2 + Codex×2 四路互相对抗审查
# 用法：adversarial-review.sh <SLOT>
# 每个 cron 调用 = 独立进程 = context 天然清零
set -euo pipefail

SLOT="${1:?need SLOT arg 0..5}"
REPO=/root/code/go/src/github.com/omeyang/XKit

# cron 环境显式 PATH
export PATH="/root/.local/share/fnm/node-versions/v24.14.1/installation/bin:/usr/local/bin:/root/.local/bin:/root/code/go/bin:/usr/bin:/bin"
export HOME=/root
# cron 非交互 shell 不加载 ~/.zshrc，必须显式放行 root + --dangerously-skip-permissions
export IS_SANDBOX=1

LOG_DIR="$REPO/.adversarial-runs"
mkdir -p "$LOG_DIR"
TS=$(date -u +%Y%m%dT%H%M%SZ)
RUNLOG="$LOG_DIR/slot${SLOT}-${TS}.log"
exec > >(tee -a "$RUNLOG") 2>&1

LOG_FILE="$REPO/docs/adversarial-review-log.md"

# 向 adversarial-review-log.md 追加失败条目（仅在真正失败路径调用）
append_failure_log() {
  local stage="$1" detail="$2"
  local date_cst; date_cst=$(TZ=Asia/Shanghai date +%Y-%m-%d)
  {
    [[ -f "$LOG_FILE" ]] || echo "# Adversarial Review Log"
    echo ""
    echo "## $date_cst slot=$SLOT TARGET=${TARGET:-unknown}"
    echo "- 状态: FAILED"
    echo "- 阶段: $stage"
    echo "- 详情: $detail"
    echo "- 运行日志: $RUNLOG"
  } >> "$LOG_FILE" 2>/dev/null || true
}

# 依赖体检：缺任何一个就 fail loud 而非静默退出
for bin in claude codex task git awk find tee rg; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "FATAL: dependency '$bin' not found in PATH=$PATH"
    append_failure_log "依赖体检" "$bin 不在 PATH"
    exit 2
  fi
done

echo "=== [$(date -Iseconds)] SLOT=$SLOT start ==="
cd "$REPO"
git fetch origin main --quiet
git checkout main --quiet
git merge --ff-only origin/main

# 未审查包清单（22 个；xelection 已于 2026-04-14 首轮归档；xtls 为新增未审查包）
PKGS=(xauth xconf xenv xplatform xtenant xdbg xcron xdlock xhealth xrun xpulsar xlog xbreaker xretry xcache xclickhouse xetcd xmongo xetcdtest xredismock xfile xtls)
N=${#PKGS[@]}
DOY=$(date -u +%j); DOY=$((10#$DOY))
IDX=$(( (DOY*6 + SLOT) % N ))
TARGET="${PKGS[$IDX]}"
echo "initial pick: IDX=$IDX TARGET=$TARGET (DOY=$DOY N=$N)"

# 3 天内已审则向后跳
if [[ -f "$LOG_FILE" ]]; then
  CUTOFF=$(date -u -d '3 days ago' +%Y-%m-%d)
  for _ in 1 2 3 4 5; do
    if awk -v t="$TARGET" -v c="$CUTOFF" '
      /^## / {cur_date=$2}
      $0 ~ ("TARGET="t"($| )") && cur_date>=c {found=1; exit}
      END {exit found?0:1}' "$LOG_FILE"; then
      IDX=$(( (IDX+1) % N )); TARGET="${PKGS[$IDX]}"
      echo "skip recent; retry -> $TARGET"
    else
      break
    fi
  done
fi

PKG_DIR=$(find "$REPO/pkg" -mindepth 2 -maxdepth 2 -type d -name "$TARGET" 2>/dev/null | head -1)
if [[ -z "$PKG_DIR" ]]; then
  echo "TARGET not found under pkg/*/; skip"
  {
    [[ -f "$LOG_FILE" ]] || echo "# Adversarial Review Log"
    echo ""
    echo "## $(TZ=Asia/Shanghai date +%Y-%m-%d) slot=$SLOT TARGET=$TARGET"
    echo "- 状态: 包不存在于 pkg/*/ 下，跳过"
  } >> "$LOG_FILE" 2>/dev/null || true
  exit 0
fi
echo "final TARGET=$TARGET PKG_DIR=$PKG_DIR"

# ===== 阶段 1：Codex 双路独立扫描（严格表格输出） =====
CODEX_A="$LOG_DIR/codex-A-${TARGET}-${TS}.md"
CODEX_B="$LOG_DIR/codex-B-${TARGET}-${TS}.md"

CODEX_OUTPUT_SPEC=$'## 输出规范（强约束）\n- **只输出最终清单，禁止输出搜索过程、工具调用、思考过程、验证说明**\n- 严格 Markdown 表格，列头：`严重度 | 文件:行号 | 根因(≤80字) | 修复建议(≤80字) | 非FP理由(≤60字)`\n- 最多 8 行\n- 无真问题就只输出一行：`无发现`\n- 严重度：FG-H（可致 panic/数据错乱/死锁/泄漏）、FG-M（契约偏离/错误丢失/竞态边缘情况）、FG-L（代码异味）\n- 只列 FG-H 和 FG-M；FG-L 忽略'

codex exec -s danger-full-access --cd "$PKG_DIR" \
  "对 XKit $TARGET 包对抗审查，读目录下所有 .go（含 doc.go / _test.go）。扫描维度：nil/typed-nil/零值契约、并发安全（mutex 边界/goroutine 泄漏/线性化缺口/atomic 顺序）、错误处理（%w / errors.Join 双 cause / 错误链）、context 传播与 nil 防御、资源清理（Close 幂等/cleanup goroutine 退出）、API 契约、跨平台 build tag。$CODEX_OUTPUT_SPEC" \
  > "$CODEX_A" 2>&1 &
CODEX_A_PID=$!

codex exec -s danger-full-access --cd "$PKG_DIR" \
  "作为 XKit $TARGET 资深复核者，独立审查所有 .go，只列你证据最充分的 FG-H/M 真问题，每条必须能举出攻击路径。$CODEX_OUTPUT_SPEC" \
  > "$CODEX_B" 2>&1 &
CODEX_B_PID=$!

# ===== 阶段 2：Claude 主编排 =====
# 它负责：启动 2 个 Explore 子代理做 Claude 侧攻守 → 等 Codex 完成 → 跨阵营对抗 → 合议 → 修复 → 推送
CLAUDE_PROMPT=$(cat <<EOF
你是 XKit 自动化对抗审查 v2 主编排器。目标包：**$TARGET**（路径 $PKG_DIR）。

## 上下文
外部已并行启动 2 个 codex exec（PID=$CODEX_A_PID / $CODEX_B_PID），输出保存到：
- $CODEX_A
- $CODEX_B

Codex 被要求严格输出 Markdown 表格，≤8 行，禁止输出过程。

## 你的强制执行流程

### 阶段 A：Claude 双代理独立扫描（与 Codex 并行）
**必须**用 Agent 工具在一条消息里并行启动 2 个 Explore 子代理：

- **Agent CA（Claude 攻方）** subagent_type=Explore, thoroughness=very thorough
  prompt：\`\`\`
  你是 XKit $TARGET 包对抗审查攻方。读 $PKG_DIR 所有 .go（含 doc.go / _test.go）。
  扫描维度：nil/typed-nil/零值契约、并发安全（mutex 边界/goroutine 泄漏/线性化缺口/atomic 顺序）、错误处理（%w、errors.Join 双 cause、错误链）、context 传播与 nil 防御、资源清理（Close 幂等、cleanup goroutine 退出）、API 契约、跨平台 build tag。
  **严格输出 Markdown 表格，列：严重度|文件:行号|根因(≤80字)|修复建议(≤80字)|非FP理由(≤60字)。最多 8 行。只 FG-H/FG-M。禁止输出过程。**
  无真问题只输出：无发现
  \`\`\`

- **Agent CB（Claude 复核/守方）** subagent_type=Explore, thoroughness=medium
  prompt：\`\`\`
  你是 XKit $TARGET 包资深复核者。独立扫一遍 $PKG_DIR 所有 .go，只列证据最充分的 FG-H/M 真问题。
  同时识别常见 false positive 模式（文档化设计决策、已有防御、公共 API 契约、业内惯例）。
  输出两个表格：
  表格 1 标题"真问题"，列：严重度|文件:行号|根因|修复|证据。
  表格 2 标题"误报识别"，列：何种线索属于 FP|为什么。
  每表 ≤6 行。禁止输出过程。
  \`\`\`

### 阶段 B：等待 Codex 完成
Claude 子代理返回后立刻跑 Bash：
\`\`\`
wait $CODEX_A_PID $CODEX_B_PID || true
\`\`\`
Read 两个 Codex 输出文件全文（文件不大，<20KB）。**如果 Codex 输出含思考过程/搜索日志（未严格遵守规范），你必须手动提取表格行，不能丢弃发现。**

### 阶段 C：**跨阵营对抗审查（核心新增）**
收集 4 份原始发现后，启动两路交叉对抗：

1. **Codex 攻击 Claude 的发现**：
   把 Agent CA + CB 列出的所有 Claude 发现拼成一个清单，用 Bash 启动：
   \`\`\`
   codex exec -s danger-full-access --cd "$PKG_DIR" "以下是 Claude 双代理列出的对抗审查发现。对每条逐行判断：(a) 真问题且证据充分；(b) false positive；(c) 证据不足需更多上下文。逐条给出你的判断和一句话理由。严格表格：原编号|Claude结论|你的判断 a/b/c|理由(≤60字)。禁止输出过程。\n\n<<CLAUDE 发现>>\n\$(printf '%s\n' 此处填入 Claude 的全部发现行)" > $LOG_DIR/codex-attack-claude-${TARGET}-${TS}.md 2>&1
   \`\`\`
   （把清单 heredoc 进 codex exec 的 prompt）

2. **Claude 攻击 Codex 的发现**：
   再用 Agent 工具启动 1 个 Explore 子代理 CC（反攻）：
   prompt：\`\`\`
   以下是 Codex 双路列出的对抗审查发现（$TARGET 包）。对每条逐行判断：(a) 真问题证据充分；(b) false positive；(c) 证据不足。Read $PKG_DIR 相关源码核实，不要轻信 Codex 论断。严格表格：原编号|Codex结论|你的判断 a/b/c|理由(≤60字)。禁止输出过程。

   <<CODEX 发现>>
   <此处填入 Codex A+B 的全部发现行>
   \`\`\`

### 阶段 D：合议
基于 4 份原始 + 2 份交叉对抗，按以下规则分类：

- **必修（高置信）**：≥2 个原始来源独立指向同一文件:行号 **且** 交叉对抗至少一方判 (a)
- **必修（单源但交叉验证通过）**：仅 1 原始来源，但对阵营交叉判 (a)
- **存疑（人工）**：交叉判 (c) 或两判相反 → 你 Read 源码做最终裁决
- **舍弃**：交叉判 (b)，或匹配 MEMORY 中已文档化"Codex false positive"模式（公共 API、已文档化设计决策、已有防御、零值可用契约）

**输出合议结果表格**（写到日志第 E 阶段），列：
编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 | 对抗结果

### 阶段 E：修复
对所有"必修"+"存疑裁决为修"的问题：Read → Edit → 写/更新测试。
修完 cd 仓库根跑 \`task pre-push\`（**禁 --no-verify**）。失败看日志修根因，最多 3 轮；3 轮仍败则 \`git restore -SW .\` 回滚，把分析写入日志退出。

### 阶段 F：提交推送
- commit 风格：\`fix($TARGET): 中文简述\`；**禁 Co-Authored-By / Claude 署名**
- 若多类修复可拆多个 commit（如 fix(...) + test(...)）
- \`git push origin main\`

### 阶段 G：日志
追加 $LOG_FILE（不存在则创建 \`# Adversarial Review Log\` 标题），写入：
\`\`\`
## \$(TZ=Asia/Shanghai date +%Y-%m-%d) slot=$SLOT TARGET=$TARGET
- 原始发现：Claude攻=N 守=N / Codex A=N B=N
- 交叉对抗：Codex攻Claude → a=N b=N c=N；Claude攻Codex → a=N b=N c=N
- 合议：必修=N 存疑=N 舍弃=N
- 修复：commit <hash1> <hash2> 或 "无发现" 或 "pre-push 失败回滚: <根因>"
- 合议表格：
  | 编号 | 严重度 | 文件:行 | 根因 | 分类 | 来源数 |
  ...
\`\`\`

## 硬约束
- Go 1.25+；中文注释英文标识符；构造器返 error 不 panic；comma-ok 断言；\`_ = expr\` 算 errcheck 违规；mock 放子包；funlen≤70
- 禁破坏性 git（reset --hard / push --force / --no-verify）
- **严禁跳过任何阶段**。即使 Codex 输出看起来是"截断/无结论"，也必须手动提取表格行进入交叉对抗
- 空包（无 .go）→ 只写日志并退出

现在开始。第一步：在一条消息里同时发起两个 Agent 工具调用（CA+CB）。
EOF
)

set +e
claude -p "$CLAUDE_PROMPT" \
  --dangerously-skip-permissions \
  --model claude-opus-4-6
CLAUDE_RC=$?
set -e

wait 2>/dev/null || true

# claude 非 0 退出时，判断是否已在合议/修复阶段写过日志；若今天没为本 slot 留下任何条目则补一条 FAILED
if [[ $CLAUDE_RC -ne 0 ]]; then
  echo "claude exited with $CLAUDE_RC"
  TODAY_CST=$(TZ=Asia/Shanghai date +%Y-%m-%d)
  if ! grep -q "^## $TODAY_CST slot=$SLOT TARGET=$TARGET" "$LOG_FILE" 2>/dev/null; then
    STDERR_TAIL=$(tail -n 20 "$RUNLOG" 2>/dev/null | tr '\n' ' ' | head -c 800)
    append_failure_log "Claude 编排器" "rc=$CLAUDE_RC; codex 原始输出: $CODEX_A / $CODEX_B; stderr tail: $STDERR_TAIL"
  fi
fi

echo "=== [$(date -Iseconds)] SLOT=$SLOT end rc=$CLAUDE_RC ==="
