#!/usr/bin/env bash
# XKit 对抗审查日对账：检查今日 15 个 slot 是否都在 .adversarial-runs/review-log.md 留下条目
# 0 条或 <15 条 → 输出告警到 stdout（cron 会邮件到 root）+ 落盘到 .adversarial-runs/daily-check.log
set -euo pipefail

REPO=/root/code/go/src/github.com/omeyang/XKit
export PATH="/root/.local/share/fnm/node-versions/v24.14.1/installation/bin:/usr/local/bin:/root/.local/bin:/root/code/go/bin:/usr/bin:/bin"
export HOME=/root

LOG_FILE="$REPO/.adversarial-runs/review-log.md"
CHECK_LOG="$REPO/.adversarial-runs/daily-check.log"
TODAY=$(TZ=Asia/Shanghai date +%Y-%m-%d)

mkdir -p "$(dirname "$CHECK_LOG")"

COUNT=0
if [[ -f "$LOG_FILE" ]]; then
  COUNT=$(grep -c "^## $TODAY slot=" "$LOG_FILE" 2>/dev/null || echo 0)
fi

EXPECTED=15
STATUS="OK"
if [[ $COUNT -lt $EXPECTED ]]; then STATUS="ALERT"; fi

MSG="[$(date -Iseconds)] $STATUS date=$TODAY entries=$COUNT/$EXPECTED"
echo "$MSG" | tee -a "$CHECK_LOG"

if [[ $STATUS == "ALERT" ]]; then
  echo "--- 今日 .adversarial-runs/ 中 slot 日志 ---" | tee -a "$CHECK_LOG"
  ls -l "$REPO/.adversarial-runs/" | grep "slot.*$(date -u -d 'yesterday' +%Y%m%d)\|slot.*$(date -u +%Y%m%d)" | tee -a "$CHECK_LOG" || true
  echo "--- log 文件今日条目 ---" | tee -a "$CHECK_LOG"
  grep -A 4 "^## $TODAY slot=" "$LOG_FILE" 2>/dev/null | tee -a "$CHECK_LOG" || echo "(无条目)" | tee -a "$CHECK_LOG"
  # cron 非 0 退出会触发邮件（如果有 mail 配置）
  exit 1
fi

exit 0
