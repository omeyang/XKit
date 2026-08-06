#!/usr/bin/env bash
# XKit docs ledger check: 阻止文档出现"记账式"演进叙述。
#
# 规则：文档只描述当前客观状态，不写"以前 A → 后来 B → 现在 C"。
# 历史演进去 CHANGELOG.md。
#
# 例外：CHANGELOG.md 不检查。
#
# 用法：
#   .githooks/docs-ledger-check.sh          # 扫描全部追踪的文档
#   task docs-ledger-check                   # 同上（推荐）

set -euo pipefail

# 记账式短语（聚焦高特异性表述，避免误伤）
pattern='以前是|以前曾|以前的版本|原本是|原本为|最初是|最初为|后来改|后来变|后来被|之前是|之前的版本|原来是|原来的实现|现在改为|现在变成|现在已改|改成了|修改为了|曾经是|曾经有过'

# 扫描范围：docs/ 下 Markdown（含顶层）+ 根目录 README
# 排除：CHANGELOG.md
# 同时包含已追踪与未追踪（新增）的文档，以便提交前即可捕获
#
# 必须同时给出 'docs/*.md' 与 'docs/**/*.md'：git pathspec 默认不带 :(glob) 魔法，
# 'docs/**/*.md' 要求至少一层中间目录，单独使用会漏掉 docs/00-index.md、
# docs/02-progress.md 等顶层文档——而 02-progress.md 正是包稳定性矩阵的单一事实源。
# 自证：git ls-files -- 'docs/**/*.md' | grep -cE '^docs/[^/]+\.md$' 为 0。
#
# `|| true` 只能作用于 grep（空匹配退出 1 属正常），不能罩住整条 pipeline——否则
# git ls-files 失败也会被吞掉，files 变空后走"无文档可扫描"分支 exit 0，门禁静默失效。
# 故先把 git 的输出单独取出并校验退出码，再做过滤。
raw=$(
  {
    git ls-files -- 'docs/*.md' 'docs/**/*.md' 'README.md' || exit 1
    git ls-files --others --exclude-standard -- 'docs/*.md' 'docs/**/*.md' 'README.md' || exit 1
  } | sort -u
) || {
  echo "docs-ledger-check: git ls-files 失败，无法确定扫描范围" >&2
  exit 2
}
mapfile -t files < <(printf '%s\n' "$raw" | grep -vE '^CHANGELOG\.md$' | grep -v '^$' || true)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "docs-ledger-check: 无文档可扫描"
  exit 0
fi

# grep 的三个退出码必须分开处理：0=命中、1=无命中、>=2=grep 自身出错。
# 原写法用 `if hits=$(grep ...)` 只区分 0 与非 0，退出 2（文件不可读、模式非法等）
# 会落到 else 分支打印 ✅ 并 exit 0——门禁自己坏掉时反而放行。
hits=$(grep -nE "$pattern" "${files[@]}" 2>&1) || rc=$?
rc=${rc:-0}
case "$rc" in
  0)
    echo "❌ docs-ledger-check: 检测到记账式表述"
    echo "   规则：文档只记当前状态；历史演进入 CHANGELOG.md"
    echo ""
    echo "$hits"
    exit 1
    ;;
  1) ;;  # 无命中，继续
  *)
    echo "docs-ledger-check: grep 执行失败（退出码 $rc），门禁无法生效" >&2
    echo "$hits" >&2
    exit "$rc"
    ;;
esac

echo "✅ docs-ledger-check: ${#files[@]} 份文档无记账式表述"
