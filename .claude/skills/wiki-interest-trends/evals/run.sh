#!/usr/bin/env bash
# Run eval scenarios headless in Claude Code on a cheap model.
#
#   evals/run.sh                      # all scenarios, 1 run each, model haiku
#   REPEAT=3 evals/run.sh astro_uk    # one scenario, 3 runs
#   MODEL=sonnet evals/run.sh         # another model
#
# Each run gets a fresh project directory with only this skill installed, so
# the agent sees nothing but SKILL.md. Results: evals/results/<timestamp>/.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL="$(dirname "$HERE")"
MODEL="${MODEL:-haiku}"
REPEAT="${REPEAT:-1}"
STAMP="$(date +%Y%m%d-%H%M%S)-$MODEL"
OUT="${OUT:-$HERE/results/$STAMP}"
# Agents must run OUTSIDE any git repository: Claude Code discovers skills from
# the repository root, so a workdir inside this repo would use the original skill.
WORKROOT="${WORKROOT:-${TMPDIR:-/tmp}/wikitrend-evals/$STAMP}"
mkdir -p "$WORKROOT"
export WIKITREND_CONTACT="${WIKITREND_CONTACT:-wikitrend-evals}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)" # absolute: runs cd into their workdir

ids=("$@")
if [ ${#ids[@]} -eq 0 ]; then
  while IFS= read -r id; do ids+=("$id"); done < <(python3 -c "import json;[print(s['id']) for s in json.load(open('$HERE/scenarios.json'))['scenarios']]")
fi

run_claude() { # <workdir> <prompt> <out.jsonl> [session-id-to-resume]
  local wd="$1" prompt="$2" out="$3" resume="${4:-}"
  local extra=()
  [ -n "$resume" ] && extra=(--resume "$resume")
  (cd "$wd" && claude -p "$prompt" --model "$MODEL" --output-format stream-json --verbose \
      --setting-sources project --permission-mode acceptEdits \
      --allowedTools "Bash" "Read" "Write" "Edit" "Skill" "Glob" "Grep" \
      --max-budget-usd 1 ${extra[@]+"${extra[@]}"} >"$out" 2>"$out.stderr")
}

for id in "${ids[@]}"; do
  prompt=$(python3 -c "import json,sys;print({s['id']:s for s in json.load(open('$HERE/scenarios.json'))['scenarios']}[sys.argv[1]]['prompt'])" "$id")
  follow=$(python3 -c "import json,sys;print({s['id']:s for s in json.load(open('$HERE/scenarios.json'))['scenarios']}[sys.argv[1]].get('followup',''))" "$id")
  for n in $(seq 1 "$REPEAT"); do
    rd="$OUT/$id-$n"
    wd="$WORKROOT/$id-$n"
    mkdir -p "$rd" "$wd/.claude/skills"
    ln -sfn "$wd" "$rd/workdir"
    rsync -a --exclude evals/results --exclude .venv "$SKILL/" "$wd/.claude/skills/wiki-interest-trends/"
    echo "▶ $id #$n"
    run_claude "$wd" "$prompt" "$rd/turn1.jsonl"
    if [ -n "$follow" ]; then
      sid=$(python3 -c "import json,sys
for l in open(sys.argv[1]):
    try: e=json.loads(l)
    except Exception: continue
    if e.get('session_id'): print(e['session_id']); break" "$rd/turn1.jsonl")
      run_claude "$wd" "$follow" "$rd/turn2.jsonl" "$sid"
    fi
    python3 "$HERE/check.py" "$id" "$rd" | tee -a "$OUT/results.jsonl"
  done
done

python3 - "$OUT/results.jsonl" <<'EOF'
import json, sys, collections
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
by = collections.defaultdict(list)
for r in rows:
    by[r["scenario"]].append(r)
print(f"\n{'scenario':24} pass   cost$   turns  tools  errors")
tot = 0
for s, rs in by.items():
    k = sum(r["pass"] for r in rs)
    tot += k
    avg = lambda f: sum(r["metrics"][f] for r in rs) / len(rs)
    print(f"{s:24} {k}/{len(rs)}  {avg('cost'):6.3f}  {avg('turns'):5.1f}  {avg('tool_calls'):5.1f}  {avg('cli_errors'):5.1f}")
    for r in rs:
        if not r["pass"]:
            print("   ✗", r["run"], "; ".join(r["failed"])[:300])
print(f"TOTAL {tot}/{len(rows)} runs passed, cost ${sum(r['metrics']['cost'] for r in rows):.2f}")
EOF
