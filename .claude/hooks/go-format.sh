#!/usr/bin/env bash
# PostToolUse (Edit|Write): форматує змінений .go-файл.
# Не блокує: помилки gofmt показуються моделі через stderr + exit 2.
set -u

file=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)

case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac

[ -f "$file" ] || exit 0

if command -v goimports >/dev/null 2>&1; then
  goimports -w "$file" 2>&1 >/dev/null || { echo "goimports failed on $file" >&2; exit 2; }
fi

out=$(gofmt -l -w "$file" 2>&1) || { echo "gofmt: $out" >&2; exit 2; }
exit 0
