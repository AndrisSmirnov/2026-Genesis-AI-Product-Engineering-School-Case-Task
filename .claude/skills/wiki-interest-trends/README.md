# wiki-interest-trends

An Agent Skill that helps B2C founders decide which topics to develop and in which languages to launch. It measures whether interest in a topic is growing in Wikipedia language editions and how far that can be trusted. The output is a chart and a one-page PDF report.

- The **agent** (even Claude Haiku 4.5) only runs 4–5 CLI commands and writes text.
- **Go code** does all the data work: Wikidata resolution, pageviews download with cache and rate limiting, share metric, robust trend, 7 trust checks, chart, PDF, and a validator that makes invented numbers impossible.

## Requirements

- bash and Go 1.22+ (`go version`). Go downloads one module (gopdf) on the first run and compiles in about 10–60 s. Nothing is stored in the skill: the compiled program lives in Go's build cache.
- HTTPS access to `wikimedia.org` and `wikidata.org`.
- Optional: `WIKITREND_CONTACT=<email or URL>`. It goes into the User-Agent, as the Wikimedia API policy asks.

## Install

Copy this directory into the skills folder of your agent:
- Claude Code: `.claude/skills/wiki-interest-trends/` in a project, or `~/.claude/skills/`;
- OpenCode, Codex and others: `.agents/skills/`.

## Try it by hand

```bash
WT=.claude/skills/wiki-interest-trends/scripts/wikitrend
$WT doctor
$WT resolve --topic "astronomy" --langs uk,pl
$WT analyze --spec wikitrend-data/runs/<run_id>/spec.json
echo -e 'Headline {{uk.verdict}}\nTrend {{uk.trend}} per year, trust {{uk.trust}}.' > n.md
$WT report --run <run_id> --narrative n.md --lang en
$WT analyze --from <run_id> --set window_months=24     # follow-up, served from cache
```

Where files go:
- runs: `./wikitrend-data/runs/<run_id>/` — `spec.json`, `result.json`, `data.json`, `chart.svg`, `report.pdf`;
- HTTP cache: `~/Library/Caches/wikitrend` (macOS) or `~/.cache/wikitrend` (Linux).

Override them with `WIKITREND_HOME` and `WIKITREND_CACHE`.

## Layout

```
SKILL.md                 instructions for the agent (golden path, parameter mapping, rules)
references/              METHODOLOGY.md, ERRORS.md, EXAMPLES.md (loaded on demand)
scripts/wikitrend        bash entry point → go run ./cmd/wikitrend
scripts/cmd/wikitrend    CLI: doctor, resolve, analyze, report, explain
scripts/internal/
  httpx    User-Agent, shared rate limit, retries on 429/5xx, file cache
  wiki     Wikidata search/entities, MediaWiki redirects, pageviews API
  spec     reproducible analysis spec; run_id = hash; --set overrides
  stats    median/MAD, Hampel, Theil–Sen, Sen CI, (seasonal) Mann–Kendall
  analysis share metric, trend, checks C1–C7, trust, ranking, placeholders
  report   narrative validator, chart (SVG + PDF), one-page PDF (Noto Sans)
dev/gen_fixtures.py      reference values from scipy/pymannkendall (dev only)
evals/                   scenarios, headless runner for Claude Code, grader
```

## Tests

```bash
cd scripts && go test ./...           # stats vs scipy, synthetic series, calibration, PDF
cd scripts && go test -short ./...    # fast subset (no simulations)
evals/run.sh                          # 9 scenarios on Claude Haiku 4.5 (needs `claude` CLI)
REPEAT=3 evals/run.sh                 # 3 runs per scenario
```

## Methodology in one paragraph

- **Metric.** The topic's articles (and their redirects) are summed per day. One-day spikes are removed with a Hampel filter. The monthly totals are divided by all human pageviews of the language edition. This share is the metric, because Wikipedia traffic as a whole is falling.
- **Trend.** Theil–Sen slope of the log share, converted to % per year, with Sen's 90% interval widened for autocorrelation.
- **Trust.** A seasonal Mann–Kendall test and six more checks (magnitude, spikes, volume, window sensitivity, the 2025 bot reclassification, basket consistency) give high / medium / low trust with reasons.
- **Calibration.** Thresholds were calibrated on synthetic series. Details: [references/METHODOLOGY.md](references/METHODOLOGY.md). Where to take it next: [ROADMAP.md](ROADMAP.md).
