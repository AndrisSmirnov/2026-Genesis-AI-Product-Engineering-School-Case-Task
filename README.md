# Wikipedia interest trends — Agent Skill

**EN** · [UA](#ua)

An [Agent Skill](https://agentskills.io/specification) for B2C founders. You ask in plain words — *"Is interest in astronomy growing in Ukrainian Wikipedia, and how far can we trust it?"* — and the agent answers from Wikipedia pageview data:

- whether interest is growing, per language, in % per year with a 90% interval;
- how far the result can be trusted (high / medium / low) and why;
- a chart and a one-page PDF report you can share.

It works on a cheap model. It was tested on **Claude Haiku 4.5** (27 runs) and on a **free OpenRouter model** in OpenCode.

## How it works

```
User question ──► AI agent reads SKILL.md ──► runs the Go CLI step by step
                                              doctor → resolve → analyze → report
                                                         │
             Wikidata (topic → articles in every language) + Wikimedia Pageviews API
                                                         │
             share of pageviews · robust trend · 7 trust checks · chart · PDF
```

**The code computes, the model orchestrates.** The model never calculates. It only runs commands, reads a short JSON (≤ 2 KB), asks the user when the topic is ambiguous, and writes text. The text of the report may contain numbers only as `{{placeholders}}`; any other digit is rejected, so the report cannot contain invented numbers.

## Quick start

Requirements: **Go 1.22+**, bash, internet access. Python is *not* needed to use the skill.

```bash
git clone https://github.com/AndrisSmirnov/2026-Genesis-AI-Product-Engineering-School-Case-Task.git
cd 2026-Genesis-AI-Product-Engineering-School-Case-Task
claude --model haiku        # or any agent that supports Agent Skills
```

Then ask, for example:

> Compare interest in cybersecurity in Polish, Czech and Romanian Wikipedia over 3 years and prepare a PDF report.

By hand, without a model:

```bash
WT=.claude/skills/wiki-interest-trends/scripts/wikitrend
$WT doctor
$WT resolve --topic "astronomy" --langs uk,pl
$WT analyze --spec wikitrend-data/runs/<run_id>/spec.json
$WT explain --run <run_id>                                   # all trust checks
$WT analyze --from <run_id> --set window_months=24           # follow-up, from cache
```

Results are written to `wikitrend-data/runs/<run_id>/`: `report.pdf`, `chart.svg`, `result.json`.

## Repository map

| Path | What |
|---|---|
| [`.claude/skills/wiki-interest-trends/`](.claude/skills/wiki-interest-trends/) | **The skill** (the deliverable): `SKILL.md`, Go code, references, evals |
| [`…/SKILL.md`](.claude/skills/wiki-interest-trends/SKILL.md) | Instructions for the agent |
| [`…/references/METHODOLOGY.md`](.claude/skills/wiki-interest-trends/references/METHODOLOGY.md) | Metric, trend, trust checks, calibration |
| [`…/ROADMAP.md`](.claude/skills/wiki-interest-trends/ROADMAP.md) | How to grow it to larger research and data volumes |
| [`…/evals/`](.claude/skills/wiki-interest-trends/evals/) | Headless eval runner (Claude Code / OpenCode) and grader |
| [`DECISIONS.md`](DECISIONS.md) | Architecture decisions, alternatives and what changed |
| [`VERIFICATION.md`](VERIFICATION.md) | How every AI-generated part was verified |
| [`docs/research/`](docs/research/) | The task and the research the design is based on |
| `CLAUDE.md`, `.claude/` | Claude Code setup used for development: rules, hooks, subagents |

## How it was verified

| Level | Evidence |
|---|---|
| Math | Every statistic matches scipy / pymannkendall reference values to 1e-9 |
| Method | Synthetic series with a known answer. With the default 36-month window: 0% false "high trust" on flat data, 82% detection of +15%/yr growth |
| Live data | The astronomy/uk result recomputed by hand from raw API responses: identical |
| Cheap model | 9 scenarios × 3 runs on Claude Haiku 4.5: 25/27 passed, 27/27 after two fixes; ≈ $1.5 in total |
| Portability | OpenCode + free NVIDIA Nemotron via OpenRouter: 3/5 scenarios passed ($0) |

```bash
make test          # Go tests incl. calibration
make eval-smoke    # 3 key scenarios on Haiku (needs the `claude` CLI)
```

## Limitations

- Pageviews show interest, not willingness to pay.
- Language ≠ country.
- Small topics and small editions get low trust.
- The 2025 Wikimedia bot reclassification may still leave residual bias.

---

<a id="ua"></a>

## UA

[Agent Skill](https://agentskills.io/specification) для засновників B2C-продуктів. Питаєш звичайними словами — *«Чи зростає інтерес до астрономії в україномовній Вікіпедії і наскільки цьому можна довіряти?»* — і агент відповідає за даними переглядів Вікіпедії:

- чи зростає інтерес у кожній мові, у % на рік з 90% інтервалом;
- наскільки цьому можна довіряти (висока / середня / низька) і чому;
- графік і PDF-звіт на одну сторінку, яким можна поділитися.

Працює на дешевій моделі. Перевірено на **Claude Haiku 4.5** (27 запусків) і на **безкоштовній моделі OpenRouter** в OpenCode.

**Принцип: код рахує, модель керує.** Модель нічого не рахує. Вона запускає команди, читає короткий JSON, перепитує користувача, якщо тема неоднозначна, і пише текст. Числа у звіті можуть бути лише плейсхолдерами `{{…}}`; будь-яку іншу цифру код відхиляє, тож вигаданих чисел у звіті бути не може.

**Швидкий старт.**
1. Потрібні **Go 1.22+**, bash та інтернет. Python для роботи навички не потрібен.
2. Клонуй репозиторій і запусти `claude --model haiku` у його папці.
3. Спитай, наприклад: *«Порівняй інтерес до кібербезпеки в польській, чеській і румунській Вікіпедії за 3 роки й підготуй PDF-звіт»*.

Ручний запуск без моделі — команди в англійському розділі вище. Результати зберігаються в `wikitrend-data/runs/<run_id>/`.

**Де що лежить:**
- навичка — `.claude/skills/wiki-interest-trends/`;
- рішення — `DECISIONS.md`;
- як перевірено кожну частину, згенеровану AI, — `VERIFICATION.md`;
- план розвитку — `ROADMAP.md` навички.

**Як перевірено:**
- статистика збігається з еталонами scipy до 1e-9;
- на синтетичних даних з відомою відповіддю: 0% хибної «високої довіри» і 82% виявлення зростання +15% на рік;
- реальний результат перераховано вручну — збігся;
- Haiku пройшов усі 27 запусків після двох виправлень;
- безкоштовна модель в OpenCode пройшла 3 сценарії з 5.

**Обмеження:**
- інтерес ≠ готовність платити;
- мова ≠ країна;
- малі теми отримують низьку довіру.
