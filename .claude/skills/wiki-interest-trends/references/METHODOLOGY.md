# Methodology

Everything below is implemented in `scripts/internal/analysis` and `scripts/internal/stats`. It is tested against scipy / pymannkendall reference values and against synthetic series with a known answer.

## Data

- **Wikimedia Analytics API (REST v1):**
  - `per-article/{lang}.wikipedia/{access}/user/{title}/daily` — the topic's articles plus their redirects, summed;
  - `aggregate/{lang}.wikipedia/{access}/user/monthly` — the whole edition.
- `agent=user` only: bots and spiders are excluded.
- A 404 means zero views.
- The current month is never used. The last complete month is last month, or the month before during the first 2 days of a month.
- **Topic → articles:** Wikidata entity (QID) → sitelinks per language, plus up to 20 main-namespace redirects per article. A basket is several QIDs.
- **HTTP:**
  - every request carries an explicit User-Agent;
  - one rate limiter, ~3 requests/s;
  - retries on 429/5xx, honouring `Retry-After`;
  - file cache. Finished periods are cached forever; the rest (Wikidata answers and data that is still changing) for 1–7 days.

## Metrics (per language, over the window, default 36 months)

1. **Spike cleaning.** A Hampel filter runs on daily topic views: ±3 days, threshold 3·1.4826·MAD, with the MAD floored at 1 view. Flagged days are replaced by the window median.
2. **Share.** Monthly topic views ÷ monthly views of the whole edition × 10⁶. This removes the general decline of Wikipedia traffic (about −8%/yr in 2025) and makes languages comparable.
3. **Trend.** Theil–Sen slope of ln(clean share) per month, converted to `exp(12·slope) − 1` (% per year).
4. **Interval.** The 90% interval of Sen (1968), as in `scipy.stats.theilslopes`. Its variance is multiplied by `(1+r)/(1−r)`, where `r` is the lag-1 autocorrelation of the residuals after removing calendar-month means (effective sample size idea of Yue & Wang, 2004; capped at r = 0.66).
5. **Direction.** Seasonal Mann–Kendall test (period 12), with the same variance factor.
6. **YoY.** (share over the last 12 months) ÷ (share over the previous 12 months) − 1, computed from 12-month sums.

## Trust checks

| # | Check | Pass when |
|---|---|---|
| C1 | direction | Mann–Kendall p < 0.05 and its sign matches Theil–Sen |
| C2 | magnitude | \|trend\| ≥ 10%/yr and the 90% interval does not include 0 |
| C3 | spikes | the trend with and without spikes differs by < max(50%, 5 pp) with the same sign; the top-3 days are < 20% of last year's views |
| C4 | volume | median ≥ 1000 views/month |
| C5 | window | the same sign when the window ends 3 months earlier and with a 24/36-month window |
| C6 | 2025 bot break | if the window includes Mar–Aug 2025: the trend without those months is similar (same rule as C3); otherwise n/a |
| C7 | basket | ≥ 2/3 of the basket articles move the same way (n/a for one article) |

Levels:
- **high** — C1, C2, C3, C4, C5 pass, and C6 or C7 passes.
- **medium** — C1 and C4 pass, and exactly one of {C2, C3, C5, C6/C7} fails.
- **low** — anything else, and always when the median is below 300 views/month.

Verdict:
- `growing` / `declining` — when C1 passes;
- `no_clear_trend` — otherwise.

The user's own criteria (`min_growth_pct`, `min_monthly_views`) do not change trust. They only set `meets_criteria` and affect how results are presented.

## Calibration (`scripts/internal/analysis/calibrate_test.go`)

200 synthetic series per class. Noise model:
- Poisson daily counts;
- a ±10% yearly cycle;
- 8% month-level AR(1) noise (φ = 0.5);
- in some classes, a −8%/yr decline of the whole edition.

| Class | 24-month window: high | 36-month window: high |
|---|---|---|
| no change | 2% | 0% |
| no change + one viral day | 2% | 0% |
| +15%/yr | 36% | 82% |
| +30%/yr | 62% | 99% |
| +30%/yr, 8 views/day | 0% | 0% |

Conclusion: 24 months is too short to confirm moderate growth. The default is therefore 36 months, and shorter windows get the `short_window` warning.

Rejected alternatives (evidence in `VERIFICATION.md` of the project):
- **Block bootstrap for the interval** — 78% coverage instead of 90%, because duplicated residuals collapse the pairwise slopes.
- **Deseasonalised plain Mann–Kendall** — 17% false "growing" on null series, because two points per calendar month overfit the seasonal means.

## Limitations

- Pageviews measure interest, not willingness to pay.
- Language ≠ country.
- AI answers in search engines are changing Wikipedia traffic in different ways for different topics.
- Articles that are new or renamed have short histories; they get the `article_new` warning.
- The 2025 bot reclassification may still leave residual bias; the share metric and check C6 reduce it but do not remove it.
