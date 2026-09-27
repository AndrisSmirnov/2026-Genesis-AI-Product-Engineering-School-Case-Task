# Roadmap: from basic questions to larger research

The principle stays the same at every step: **code computes, the model orchestrates**. Every new capability is a CLI flag or command with compact JSON output, a methodology note, a test with a known answer, and an eval scenario. Each step is driven by what the evals and real questions show to be missing.

## v1: now

- 1–8 languages, one topic or a basket of up to ~5 Wikidata entities.
- Share metric, Theil–Sen trend with a 90% interval, 7 trust checks, three trust levels.
- A one-page PDF and an SVG chart.
- Spec-driven follow-ups (`--from --set`) and a file cache: a repeated query makes no network requests.
- 9 eval scenarios on Claude Haiku 4.5.

## v2: richer questions, same scale

- **Scoring by the user's criteria.** Add a weighted "promise" score (growth, volume, trust, audience size by unique devices) with weights stored in the spec. It lets the user ask "rank these 10 languages for my course".
- **Many topics × many languages in one spec**: an `analyze` matrix plus a heatmap in the PDF. Stdout returns only the top N; everything else goes to `result.json`.
- **Extra signals from the same API:**
  - editors and edits per article, as a supply-side signal;
  - unique devices per edition, as audience size;
  - `top-by-country` to show that "language ≠ country".
- **Basket assistant**: `suggest --qid Q…` proposes related articles (Wikidata subclass / "part of" relations and links), which the user confirms.
- **Parallel download**: goroutines with one shared rate limiter (≤ 200 req/min with a proper UA). SQLite (`modernc.org/sqlite`, pure Go) replaces the file cache once there are thousands of entries.
- **Incremental updates**: finished months are immutable, so only new months are downloaded.

## v3: large data

- For hundreds or thousands of articles, switch from the API to the monthly **pageview dumps** (`dumps.wikimedia.org/other/pageview_complete`). They are downloaded once, filtered by project, and aggregated into columnar files (Parquet via a pure-Go library, or DuckDB outside the skill).
- **Semantic topic grouping**: embed article titles/abstracts and cluster them into topics (Qdrant or an in-process index) to answer "what is rising in education-related topics in Polish?".
- **Change-point detection** (e.g. PELT) to report *when* interest changed, not only the average trend.
- **Cross-checks**: Google Trends (no official free API — check the terms) and the app's own funnel data, when the user provides them.

## v4: monitoring as a service

- Scheduled runs (cron or Claude Code routines) that watch a portfolio of topics × languages and alert on significant changes.
- Only at this point is a separate Go service justified:
  - **PostgreSQL** for specs, history and results;
  - **Redis** for the cache and the global rate limiter;
  - **NATS** for the queue of collection jobs.
- The skill then becomes a thin client of that service. Until then, external services would break the "self-contained skill" requirement and add operational cost without benefit for a single user.

## How each iteration is done

1. Collect questions the current version answers badly (from users and from evals).
2. Add an eval scenario and a test with a known answer **before** the code.
3. Implement in the Go CLI; keep stdout ≤ 2 KB and add a `next_step`.
4. Update SKILL.md, keeping it short. Details go to `references/`.
5. Run the evals on the cheap model (3 runs per scenario). Ship only when they do not regress.
