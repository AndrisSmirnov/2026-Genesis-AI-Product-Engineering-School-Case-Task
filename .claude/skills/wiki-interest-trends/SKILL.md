---
name: wiki-interest-trends
description: Measures whether public interest in a topic is growing in different Wikipedia language editions, how far that can be trusted, and produces a chart and a one-page PDF report. Use when a founder or product team asks about interest, demand, popularity or trends of a topic, course, niche or language market, compares languages/countries for a launch or localization, or asks which audiences to research next — even if Wikipedia is not mentioned.
license: MIT
compatibility: Requires bash, Go 1.22+ on PATH (code runs via `go run`; the first run downloads one module and compiles, ~10-60 s) and HTTPS access to wikimedia.org and wikidata.org. Not usable in sandboxes without network.
metadata:
  version: 0.1.0
---

# Wikipedia interest trends

All data work is done by the `wikitrend` CLI. **You never compute numbers or write analysis code yourself.** You run commands, read their short JSON, ask the user when needed, and write text.

`WT` below means the absolute path `<this skill's directory>/scripts/wikitrend`. Always call this script from the directory you are working in: do not `cd` into the skill and do not call `go run` yourself. Every JSON output has a `next_step` field with the exact next command: follow it.

## Golden path

1. **Check the environment**: `WT doctor`
   - The first call compiles the code and can take up to a minute. That is normal, so wait.
   - `GO_MISSING` or `NETWORK` → tell the user what is missing and stop.
2. **Find the articles**: `WT resolve --topic "<topic, English name works best>" --langs <codes>`
   - `status: needs_choice` → show the candidates to the user and ask which one they mean. **Never guess.** Then run `WT resolve --qid <QID> --langs <codes>`.
   - Broad topic (for example "learning English") → make a basket of 2–4 concrete topics separated by `;`: `--topic "IELTS; TOEFL; English as a second language"`. Tell the user which topics you used.
   - `missing` or `skipped_topics` → tell the user: there is no article in that language (a possible unfilled niche).
3. **Analyze**: `WT analyze --spec <spec path from resolve>`
4. **Answer in chat**:
   - Use only numbers printed in the JSON; copy them exactly.
   - For each language give the verdict, the trend per year with its interval, and the trust level with its reasons.
   - If `warnings` are present, mention them. Do not add reasons or warnings that are not in the output.
   - Always add: *interest ≠ willingness to pay; language ≠ country*.
   - Even if the user asks for "just a number", give that number **together with its trust level** in one sentence. For example: "−47.7% per year (trust: low, because of low volume)".
5. **Report**: when the user wants a report, PDF or something shareable, or the question asks for a "short report" or "звіт":
   1. Write `narrative.md` at the path given in `next_step`. Format:
      - line 1: headline, one sentence, under 140 characters;
      - then 2–3 short paragraphs, under 1100 characters in total.
   2. **Do not type a single digit.** Every number must be a placeholder such as `{{uk.trend}}` (the list is in `placeholders`). Write words instead of numbers ("two years" → `{{period.months}}` months).
   3. Run `WT report --run <run_id> --narrative <path> --lang <uk|en>`.
   4. `RAW_NUMBER` / `UNKNOWN_PLACEHOLDER` / `NARRATIVE_TOO_LONG` → fix the text and rerun. Do not change the numbers.
   5. Give the user the `pdf` path.
6. **Follow-up questions** reuse the previous run (data comes from the cache):
   `WT analyze --from <run_id> --set <key>=<value> [--set ...]`

## Mapping the user's words to parameters

| User says | Parameter |
|---|---|
| "last 2 years" | `--months 24` in resolve, or `--set window_months=24` (the output warns that 24 months is short) |
| "last 3 years" / nothing specified | default is 36 months |
| "without mobile" / "desktop only" | `--set access=desktop` |
| "only mobile web" | `--set access=mobile-web` |
| "growth of at least 15%/year counts as promising" | `--min-growth 15` in resolve, or `--set min_growth_pct=15` |
| "at least 5000 views a month" | `--min-views 5000`, or `--set min_monthly_views=5000` |
| "up to June 2026" | `--end 2026-06`, or `--set end=2026-06` |
| a new language added | run `resolve` again with `--qid <same QIDs> --langs <all codes>` |

Language codes: Ukrainian `uk`, Polish `pl`, Czech `cs`, Slovak `sk`, English `en`, German `de`, French `fr`, Spanish `es`, Portuguese `pt`, Italian `it`, Romanian `ro`, Hungarian `hu`, Turkish `tr`, Russian `ru`, Vietnamese `vi`, Indonesian `id`, Japanese `ja`, Korean `ko`, Arabic `ar`, Hindi `hi`.

## How to read the results

- **`interval_90pct`** is a 90% interval (not 95%). Call it "90% interval".
- **Metric: `share_trend_pct_per_year`.** It is the change of the topic's *share* of all human pageviews in that language edition, per year. It is not raw views, because Wikipedia traffic as a whole is falling.
- **`verdict`**:
  - `growing` / `declining` — the direction is statistically clear;
  - `no_clear_trend` — the change is within noise.
- **`trust`** (high / medium / low) comes from 7 checks: direction, size of the change, spikes, volume, sensitivity to the time window, the 2025 bot reclassification, and basket consistency. The `reasons` field says why. `WT explain --run <run_id>` shows every check.
- **`ranking`** orders the languages: growing first, then by trust, then by trend. `meets_criteria` applies the user's own thresholds.
- **Recommending audiences:** prefer `growing` with high or medium trust. Say clearly when everything is `low` — then there is no basis for a decision.

## Rules

- Never write scripts, never fetch Wikipedia yourself, never read `data.json`.
- If a command returns `status: error`, do what its `next_step` says. On `RATE_LIMITED`, wait `retry_after_s` seconds and rerun the same command.
- Do not present low-trust results as findings; call them weak signals.
- Details:
  - [references/METHODOLOGY.md](references/METHODOLOGY.md) — formulas and thresholds;
  - [references/ERRORS.md](references/ERRORS.md) — all error codes;
  - [references/EXAMPLES.md](references/EXAMPLES.md) — example sessions.
