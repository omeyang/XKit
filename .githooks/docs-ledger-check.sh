#!/usr/bin/env bash
# XKit docs ledger check: 阻止文档出现"记账式"演进叙述。
#
# 规则：文档只描述当前客观状态，不写"以前 A → 后来 B → 现在 C"。
# 历史演进去 CHANGELOG.md。
#
# 例外：CHANGELOG.md 不检查。
#
# 用法：
#   .githooks/docs-ledger-check.sh          # 扫描工作树中的使用文档
#   task docs-ledger-check                   # 同上（推荐）

set -euo pipefail

# 记账式短语（聚焦高特异性表述，避免误伤）
pattern='以前是|以前曾|以前的版本|原本是|原本为|最初是|最初为|后来改|后来变|后来被|之前是|之前的版本|原来是|原来的实现|现在改为|现在变成|现在已改|改成了|修改为了|曾经是|曾经有过'

# 检查工作树中当前存在的使用文档，包括新文件和待提交的修改。
paths=$(mktemp)
trap 'rm -f -- "$paths"' EXIT
git ls-files -z --cached --others --exclude-standard -- 'docs/*.md' README.md llms.txt tests/README.md >"$paths" || {
  echo "docs-ledger-check: git ls-files 失败，无法确定扫描范围" >&2
  exit 2
}
files=()
while IFS= read -r -d '' path; do
  if [[ -e "$path" || -L "$path" ]]; then
    files+=("$path")
  fi
done < "$paths"

if [[ ${#files[@]} -eq 0 ]]; then
  echo "docs-ledger-check: 无文档可扫描"
  exit 0
fi

# grep 的三个退出码必须分开处理：0=命中、1=无命中、>=2=grep 自身出错。
# 原写法用 `if hits=$(grep ...)` 只区分 0 与非 0，退出 2（文件不可读、模式非法等）
# 会落到 else 分支打印 ✅ 并 exit 0——门禁自己坏掉时反而放行。
hits=$(grep -nHE -- "$pattern" "${files[@]}" 2>&1) || rc=$?
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
