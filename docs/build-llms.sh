#!/usr/bin/env bash
# 将当前版本的包文档、工程约定及测试使用说明汇编为 llms-full.txt。
set -euo pipefail
cd "$(dirname "$0")/.."
export LC_ALL=C
shopt -s nullglob

files=(README.md llms.txt docs/00-index.md)
for file in docs/01-decisions/[0-9]*.md; do
  [[ $file == *-template.md ]] || files+=("$file")
done
files+=(docs/02-progress.md docs/03-conventions/[0-9]*.md
        docs/04-packages/00-index.md docs/04-packages/[0-9]*/[0-9]*.md
        docs/05-concepts/[0-9]*.md docs/06-patterns/[0-9]*.md
        docs/07-glossary.md tests/README.md)

tmp=$(mktemp ./llms-full.txt.XXXXXX)
trap 'rm -f -- "$tmp"' EXIT
{
  printf '# XKit 使用文档\n\n'
  for file in "${files[@]}"; do
    printf '<!-- ===== file: %s ===== -->\n\n' "$file"
    cat -- "$file"
    printf '\n\n---\n\n'
  done
} > "$tmp"
chmod +r -- "$tmp"
mv -f -- "$tmp" llms-full.txt
printf '已将 %d 份文档写入 llms-full.txt\n' "${#files[@]}"
