#!/usr/bin/env bash
# 生成 llms-full.txt: 把 README + 全部 docs/*.md 拼接成单文件，供 LLM 一次性吞入。
# 用法: bash scripts/gen-llms-full.sh
# 也在 Taskfile.yml: task docs-llms

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

OUT="llms-full.txt"

# 拼接顺序（按知识库逻辑层次）
# 1. 入口：README + llms.txt + CHANGELOG
# 2. 顶层主分区：00~07
# 3. ADR（决策）
# 4. packages（按域）
# 5. concepts（概念）
# 6. patterns（模式）
# 7. glossary（术语）

build_file_list() {
  printf '%s\n' \
    README.md \
    llms.txt \
    CHANGELOG.md \
    CLAUDE.md

  # 顶层主分区
  printf '%s\n' docs/00-index.md docs/02-progress.md

  # ADR (按编号顺序)
  printf '%s\n' docs/01-decisions/00-index.md
  ls docs/01-decisions/*.md 2>/dev/null \
    | grep -v -E '(00-index|template)\.md$' \
    | sort

  # Conventions
  printf '%s\n' docs/03-conventions/01-api.md \
                docs/03-conventions/02-contributing.md

  # Packages：顶层索引 → 各域索引 → 各包
  printf '%s\n' docs/04-packages/00-index.md
  for domain in 01-business 02-config 03-context 04-debug 05-distributed 06-internal 07-lifecycle 08-mq 09-observability 10-resilience 11-security 12-storage 13-testkit 14-util; do
    [ -f "docs/04-packages/$domain/00-index.md" ] && printf '%s\n' "docs/04-packages/$domain/00-index.md"
    find "docs/04-packages/$domain" -name '*.md' -not -name '00-index.md' 2>/dev/null | sort
  done

  # Concepts
  printf '%s\n' docs/05-concepts/00-index.md
  find docs/05-concepts -name '*.md' -not -name '00-index.md' 2>/dev/null | sort

  # Patterns
  printf '%s\n' docs/06-patterns/00-index.md
  find docs/06-patterns -name '*.md' -not -name '00-index.md' 2>/dev/null | sort

  # Glossary
  printf '%s\n' docs/07-glossary.md
}

FILES=()
while IFS= read -r f; do
  FILES+=("$f")
done < <(build_file_list)

{
  printf '# XKit 全文文档汇编\n\n'
  printf '> 自动生成；勿手工编辑。来源：%d 个 markdown 文件。\n' "${#FILES[@]}"
  printf '> 生成时间（UTC）：%s\n\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf -- '---\n\n'

  for f in "${FILES[@]}"; do
    if [ ! -f "$f" ]; then
      printf '<!-- skip: %s (missing) -->\n\n' "$f" >&2
      continue
    fi
    printf '<!-- ===== file: %s ===== -->\n\n' "$f"
    cat "$f"
    printf '\n\n---\n\n'
  done
} > "$OUT"

wc -l "$OUT" | awk '{printf "generated %s with %s lines\n", $2, $1}'
wc -c "$OUT" | awk '{printf "size: %d bytes (~%.1f KB)\n", $1, $1/1024}'
