# Example sessions

`WT` = `<skill dir>/scripts/wikitrend`.

## 1. One topic, one language, "can we trust it?"

User: *"We are thinking of adding an astronomy course. Is interest growing in Ukrainian Wikipedia, and how far can we trust that?"*

```
WT doctor
WT resolve --topic "astronomy" --langs uk
WT analyze --spec wikitrend-data/runs/r_…/spec.json
```

Answer in chat, using only the numbers from the JSON:
- verdict (`declining`), trend per year with its interval;
- trust (`low`) with its reasons (low volume);
- warnings (2025 bot reclassification);
- interest ≠ willingness to pay.

## 2. Two languages, "last two years"

User: *"Compare the growth of interest in intermittent fasting in Polish and Czech Wikipedia over the last two years."*

```
WT resolve --topic "intermittent fasting" --langs pl,cs --months 24
```

The output has `missing: {"pl": [...]}`, which means Polish Wikipedia has no such article. Say so; it can be a sign of an unfilled niche. Then `WT analyze --spec …`. There will be a `short_window` warning; mention that 24 months is short.

## 3. Broad topic → basket, report, ranking

User: *"We are building a language-learning app. Compare interest in learning English in our languages (uk, pl, tr, vi, id) and prepare a short report: which audiences to research next and why?"*

```
WT resolve --topic "IELTS; TOEFL; English as a second language" --langs uk,pl,tr,vi,id
WT analyze --spec …
```

Write `narrative.md`. Every number is a placeholder:

```
{{rank.1}} and {{rank.2}} are the audiences to research next
Over {{period.months}} months the share of views of English-learning articles was {{vi.verdict}} in Vietnamese ({{vi.trend}} per year, interval {{vi.trend_ci}}, trust {{vi.trust}}) …
The remaining editions show weak signals: …
```

Then:

```
WT report --run r_… --narrative wikitrend-data/runs/r_…/narrative.md --lang en
```

## 4. Follow-up questions

- *"And for 3 years, without mobile?"*
  → `WT analyze --from r_… --set window_months=36 --set access=desktop`
- *"Growth of at least 20% a year counts as promising for us."*
  → `WT analyze --from r_… --set min_growth_pct=20` (see `meets_criteria`)
- *"Add German."*
  → `WT resolve --qid Q490396,Q487425 --langs uk,pl,tr,vi,id,de`

## 5. Ambiguous topic

`WT resolve --topic "Mercury" --langs pl,cs` → `status: needs_choice` with planet / chemical element / god / … Ask the user which one they mean, then `WT resolve --qid Q308 --langs pl,cs`.
