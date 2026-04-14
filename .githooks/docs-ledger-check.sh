#!/usr/bin/env bash
# XKit docs ledger check: 阻止文档出现"记账式"演进叙述。
#
# 规则：文档只描述当前客观状态，不写"以前 A → 后来 B → 现在 C"。
# 历史演进去 CHANGELOG.md；审查过程去 docs/_archive/。
#
# 例外：CHANGELOG.md、docs/_archive/** 不检查。
#
# 用法：
#   .githooks/docs-ledger-check.sh          # 扫描全部追踪的文档
#   task docs-ledger-check                   # 同上（推荐）

set -euo pipefail

# 记账式短语（聚焦高特异性表述，避免误伤）
pattern='以前是|以前曾|以前的版本|原本是|原本为|最初是|最初为|后来改|后来变|后来被|之前是|之前的版本|原来是|原来的实现|现在改为|现在变成|现在已改|改成了|修改为了|曾经是|曾经有过'

# 扫描范围：docs/ 下 Markdown + 根目录 README
# 排除：docs/_archive/**、CHANGELOG.md
# 同时包含已追踪与未追踪（新增）的文档，以便提交前即可捕获
mapfile -t files < <(
  {
    git ls-files -- 'docs/**/*.md' 'README.md'
    git ls-files --others --exclude-standard -- 'docs/**/*.md' 'README.md'
  } | sort -u | grep -vE '^(CHANGELOG\.md|docs/_archive/)' || true
)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "docs-ledger-check: 无文档可扫描"
  exit 0
fi

if hits=$(grep -nE "$pattern" "${files[@]}" 2>/dev/null); then
  echo "❌ docs-ledger-check: 检测到记账式表述"
  echo "   规则：文档只记当前状态；历史去 CHANGELOG.md，审查过程去 docs/_archive/"
  echo ""
  echo "$hits"
  exit 1
fi

echo "✅ docs-ledger-check: ${#files[@]} 份文档无记账式表述"
