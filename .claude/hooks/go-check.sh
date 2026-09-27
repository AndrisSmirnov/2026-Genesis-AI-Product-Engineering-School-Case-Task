#!/usr/bin/env bash
# Stop: go vet + go test -short у кожному Go-модулі проєкту.
# Exit 2 змушує Claude продовжити й виправити. Поки модулів немає, нічого не робить.
set -u

input=$(cat)
# Не зациклюємось: якщо Claude уже продовжує через цей хук, лише попереджаємо.
active=$(printf '%s' "$input" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("stop_hook_active", False))' 2>/dev/null)

root="${CLAUDE_PROJECT_DIR:-$(pwd)}"
mods=$(find "$root" -name go.mod -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null)
[ -z "$mods" ] && exit 0

export CGO_ENABLED=0
fail=0
report=""
for mod in $mods; do
  dir=$(dirname "$mod")
  if ! out=$(cd "$dir" && go vet ./... 2>&1); then
    fail=1; report+=$'\n'"[go vet] $dir"$'\n'"$out"
  fi
  if ! out=$(cd "$dir" && go test -short ./... 2>&1); then
    fail=1; report+=$'\n'"[go test] $dir"$'\n'"$(printf '%s' "$out" | tail -n 40)"
  fi
done

if [ "$fail" -eq 1 ]; then
  if [ "$active" = "True" ]; then
    echo "go vet/test still failing (not blocking again):$report" >&2
    exit 0
  fi
  echo "go vet/test failed — fix before finishing:$report" >&2
  exit 2
fi
exit 0
