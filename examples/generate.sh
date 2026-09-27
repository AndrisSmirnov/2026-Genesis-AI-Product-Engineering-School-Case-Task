#!/usr/bin/env bash
# Regenerate the example gallery: each case is asked to Claude Haiku 4.5 in
# Claude Code, exactly like a real user would, in a fresh directory outside
# this repository. Saves the agent's answer, the PDF, a PNG preview and the chart.
#   examples/generate.sh            # all cases
#   examples/generate.sh 04-large-language-models-growing-high-trust   # one case
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL="$ROOT/.claude/skills/wiki-interest-trends"
OUT="$ROOT/examples"
WORK="${TMPDIR:-/tmp}/wikitrend-examples"
export WIKITREND_CONTACT="${WIKITREND_CONTACT:-wikitrend-examples}"

cases=(
"01-astronomy-ukrainian-declining-low-trust|Ми думаємо додати курс з астрономії до освітнього застосунку. Чи зростає інтерес до цієї теми в україномовній Wikipedia, і наскільки цьому зростанню можна довіряти? Підготуй PDF-звіт.|"
"02-intermittent-fasting-polish-czech-missing-article|Порівняй зростання інтересу до інтервального голодування в польськомовній та чеськомовній Wikipedia за останні два роки. Підготуй короткий PDF-звіт.|"
"03-learning-english-5-languages-basket|Ми створюємо застосунок для вивчення мов. Порівняй інтерес до вивчення англійської у мовних розділах uk, pl, tr, vi, id та підготуй короткий PDF-звіт: які аудиторії варто дослідити наступними й чому?|"
"04-large-language-models-growing-high-trust|Ми робимо курс про великі мовні моделі (LLM). Чи зростає інтерес до цієї теми в польській, німецькій та українській Вікіпедії? Підготуй PDF-звіт.|"
"05-pickleball-growing-but-low-trust|Чи варто запускати застосунок для піклболу в Україні та Німеччині? Подивись, чи росте інтерес в українській і німецькій Вікіпедії, і зроби PDF-звіт.|"
"06-cybersecurity-custom-criteria|Для нас тема перспективна, якщо частка переглядів зростає щонайменше на 20% на рік і має не менше 3000 переглядів на місяць. Які з мовних розділів es, pt, it підходять для курсу з кібербезпеки? Підготуй PDF-звіт.|"
"07-ambiguous-topic-mercury-asks-user|Чи зростає інтерес до Меркурія в польській і чеській Вікіпедії?|"
"08-follow-up-desktop-2-years-from-cache|Чи росте інтерес до великих мовних моделей у німецькій Вікіпедії?|А якщо рахувати лише десктоп і за останні 2 роки?"
"09-rust-report-in-english|Prepare a one-page PDF report: is interest in the Rust programming language growing in German and French Wikipedia?|"
"10-solar-eclipse-spikes-are-not-growth|Чи росте інтерес до сонячних затемнень у німецькій і польській Вікіпедії? Наскільки цьому можна довіряти? Підготуй PDF-звіт.|"
)

ask() { # <workdir> <prompt> <out.jsonl> [resume]
  local extra=(); [ -n "${4:-}" ] && extra=(--resume "$4")
  (cd "$1" && claude -p "$2" --model haiku --output-format stream-json --verbose \
     --setting-sources project --permission-mode acceptEdits \
     --allowedTools Bash Read Write Edit Skill Glob Grep --max-budget-usd 1 \
     ${extra[@]+"${extra[@]}"} >"$3" 2>/dev/null)
}
answer() { python3 -c "import json,sys
for l in open(sys.argv[1]):
    try: e=json.loads(l)
    except Exception: continue
    if e.get('type')=='result': print(e.get('result',''))" "$1"; }

for c in "${cases[@]}"; do
  IFS='|' read -r id q1 q2 <<<"$c"
  [ $# -gt 0 ] && [[ " $* " != *" $id "* ]] && continue
  echo "▶ $id"
  wd="$WORK/$id"; rm -rf "$wd"; mkdir -p "$wd/.claude/skills" "$OUT/$id"
  rsync -a --exclude evals/results --exclude .venv "$SKILL/" "$wd/.claude/skills/wiki-interest-trends/"
  ask "$wd" "$q1" "$wd/t1.jsonl"
  { echo "# $id"; echo; echo "**User:** $q1"; echo; echo "**Agent (Claude Haiku 4.5):**"; echo; answer "$wd/t1.jsonl"; } > "$OUT/$id/answer.md"
  if [ -n "$q2" ]; then
    sid=$(python3 -c "import json,sys
for l in open(sys.argv[1]):
    try: e=json.loads(l)
    except Exception: continue
    if e.get('session_id'): print(e['session_id']); break" "$wd/t1.jsonl")
    ask "$wd" "$q2" "$wd/t2.jsonl" "$sid"
    { echo; echo "---"; echo; echo "**User:** $q2"; echo; echo "**Agent:**"; echo; answer "$wd/t2.jsonl"; } >> "$OUT/$id/answer.md"
  fi
  pdf=$(ls -t "$wd"/wikitrend-data/runs/*/report.pdf 2>/dev/null | head -1)
  if [ -n "$pdf" ]; then
    cp "$pdf" "$OUT/$id/report.pdf"
    cp "$(dirname "$pdf")/chart.svg" "$OUT/$id/chart.svg"
    pdftoppm -png -r 70 -singlefile "$pdf" "$OUT/$id/report" 2>/dev/null
  else
    chart=$(ls -t "$wd"/wikitrend-data/runs/*/chart.svg 2>/dev/null | head -1)
    [ -n "$chart" ] && cp "$chart" "$OUT/$id/chart.svg"
  fi
  ls "$OUT/$id"
done
