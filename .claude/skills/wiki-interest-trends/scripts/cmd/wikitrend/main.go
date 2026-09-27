// Command wikitrend is the CLI the agent calls. Every command prints one
// compact JSON object to stdout (≤ ~2 KB) with a next_step hint; bulky data
// goes to files under the runs directory.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"wikitrend/internal/analysis"
	"wikitrend/internal/cache"
	"wikitrend/internal/httpx"
	"wikitrend/internal/out"
	"wikitrend/internal/report"
	"wikitrend/internal/spec"
	"wikitrend/internal/wiki"
)

type env struct {
	bin, cwd, home, runs, cacheDir, contact string
	now                                     time.Time
}

// rel shortens paths under the caller's directory (fewer tokens for the model).
func (e env) rel(p string) string {
	if r, err := filepath.Rel(e.cwd, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

// abs resolves a user-given path against the directory the agent called us
// from (the wrapper changes into the skill directory before `go run`).
func (e env) abs(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.cwd, p)
}

func loadEnv() env {
	cwd := os.Getenv("WIKITREND_CWD")
	if cwd == "" {
		cwd, _ = os.Getwd()
		// Called as `cd <skill>/scripts && go run …` instead of the wrapper:
		// keep run data in the project, never inside the skill directory.
		for _, marker := range []string{"/.claude/skills/", "/.agents/skills/"} {
			if i := strings.Index(filepath.ToSlash(cwd), marker); i >= 0 {
				cwd = cwd[:i]
				break
			}
		}
	}
	home := os.Getenv("WIKITREND_HOME")
	if home == "" {
		home = filepath.Join(cwd, "wikitrend-data")
	} else if !filepath.IsAbs(home) {
		home = filepath.Join(cwd, home)
	}
	cacheDir := os.Getenv("WIKITREND_CACHE")
	if cacheDir == "" {
		if d, err := os.UserCacheDir(); err == nil {
			cacheDir = filepath.Join(d, "wikitrend")
		} else {
			cacheDir = filepath.Join(home, "cache")
		}
	}
	contact := os.Getenv("WIKITREND_CONTACT")
	if contact == "" {
		contact = "agent skill wiki-interest-trends; set WIKITREND_CONTACT"
	}
	bin := os.Getenv("WIKITREND_BIN")
	if bin == "" {
		bin = "wikitrend"
	}
	return env{bin: bin, cwd: cwd, home: home, runs: filepath.Join(home, "runs"), cacheDir: cacheDir, contact: contact, now: time.Now()}
}

func (e env) client() (*wiki.Client, *httpx.Client) {
	c, err := cache.New(e.cacheDir)
	if err != nil {
		out.Fail(out.Errf("CACHE", "Set WIKITREND_CACHE to a writable directory.", "cannot create cache dir %s: %v", e.cacheDir, err))
	}
	h := httpx.New(c, e.contact)
	return &wiki.Client{H: h}, h
}

func (e env) runDir(id string) string { return filepath.Join(e.runs, id) }

func main() {
	if len(os.Args) < 2 {
		out.Fail(out.Errf("BAD_ARGS", "Start with: doctor", "usage: wikitrend doctor|resolve|analyze|report|explain [flags]"))
	}
	e := loadEnv()
	args := os.Args[2:]
	switch os.Args[1] {
	case "doctor":
		doctor(e, args)
	case "resolve":
		resolve(e, args)
	case "analyze":
		analyze(e, args)
	case "report":
		reportCmd(e, args)
	case "explain":
		explain(e, args)
	default:
		out.Fail(out.Errf("BAD_ARGS", "Commands: doctor, resolve, analyze, report, explain.", "unknown command %q", os.Args[1]))
	}
}

func parse(fs *flag.FlagSet, args []string) {
	fs.SetOutput(new(strings.Builder))
	if err := fs.Parse(args); err != nil {
		var names []string
		fs.VisitAll(func(f *flag.Flag) { names = append(names, "--"+f.Name) })
		out.Fail(out.Errf("BAD_ARGS", "Allowed flags: "+strings.Join(names, " "), "%v", err))
	}
}

// ---------------- doctor ----------------

func doctor(e env, args []string) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.Bool("warmup", false, "compile and exit (the wrapper already compiled us)")
	parse(fs, args)
	res := map[string]any{"status": "ok", "go": runtime.Version(), "cache_dir": e.cacheDir, "runs_dir": e.runs}
	if err := os.MkdirAll(e.runs, 0o755); err != nil {
		out.Fail(out.Errf("RUNS_DIR", "Set WIKITREND_HOME to a writable directory.", "cannot create %s: %v", e.runs, err))
	}
	h := httpx.New(nil, e.contact) // no cache: really test the network
	h.MaxTries = 1
	checks := map[string]string{
		"wikimedia": "https://wikimedia.org/api/rest_v1/metrics/pageviews/aggregate/en.wikipedia/all-access/user/monthly/2025010100/2025020100",
		"wikidata":  "https://www.wikidata.org/w/api.php?action=wbgetentities&ids=Q333&props=info&format=json",
	}
	net := map[string]string{}
	failed := false
	for name, u := range checks {
		if st, _, err := h.Get(u, 0); err != nil || st != 200 {
			net[name] = fmt.Sprintf("FAIL (%v, status %d)", err, st)
			failed = true
		} else {
			net[name] = "ok"
		}
	}
	res["network"] = net
	if failed {
		out.Fail(&out.Error{Code: "NETWORK", Hint: "Cannot reach Wikimedia/Wikidata over HTTPS.", NextStep: "Tell the user the skill needs internet access to wikimedia.org and wikidata.org. Do not invent data.", Details: net})
	}
	res["next_step"] = fmt.Sprintf(`%s resolve --topic "<topic>" --langs <codes, e.g. uk,pl>`, e.bin)
	out.Print(res)
}

// ---------------- resolve ----------------

func resolve(e env, args []string) {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	topic := fs.String("topic", "", `topic in any language; "a; b; c" = basket of related topics`)
	langsF := fs.String("langs", "", "language codes, comma-separated (uk,pl,cs)")
	qidF := fs.String("qid", "", "Wikidata QID(s); several = topic basket (Q1,Q2)")
	months := fs.Int("months", 0, "analysis window in months (default 36)")
	end := fs.String("end", "", "last month YYYY-MM (default: last complete month)")
	access := fs.String("access", "all-access", "all-access|desktop|mobile-web|mobile-app")
	minGrowth := fs.Float64("min-growth", spec.DefaultCriteria.MinGrowthPct, "user criterion: min trend %/yr")
	minViews := fs.Float64("min-views", spec.DefaultCriteria.MinMonthlyViews, "user criterion: min median views/month")
	maxRedirects := fs.Int("max-redirects", 20, "redirects per article to include")
	parse(fs, args)

	langs := spec.SplitList(*langsF)
	if len(langs) == 0 || (*topic == "" && *qidF == "") {
		out.Fail(out.Errf("BAD_ARGS", fmt.Sprintf(`Example: %s resolve --topic "astronomy" --langs uk`, e.bin), "--langs and --topic (or --qid) are required"))
	}
	if len(langs) > 8 {
		out.Fail(out.Errf("BAD_ARGS", "Split the question into groups of up to 8 languages.", "too many languages (%d > 8)", len(langs)))
	}
	w, _ := e.client()
	var qids, skipped []string
	var auto []wiki.Candidate
	if *qidF != "" {
		for _, q := range strings.Split(*qidF, ",") {
			if q = strings.ToUpper(strings.TrimSpace(q)); q != "" {
				qids = append(qids, q)
			}
		}
	} else {
		// "a; b; c" = a basket of several topics, each resolved on its own.
		subs := strings.Split(*topic, ";")
		for _, sub := range subs {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			hits, err := w.Search(sub, uniq(append([]string{"en"}, langs...)), 7)
			if err != nil {
				out.Fail(err)
			}
			cands, err := w.Entities(exactFirst(hits), langs, langs[0])
			if err != nil {
				out.Fail(err)
			}
			chosen, alts, status := choose(cands, langs)
			switch status {
			case "not_found":
				if len(subs) > 1 {
					skipped = append(skipped, sub)
					continue
				}
				out.Fail(out.Errf("TOPIC_NOT_FOUND", "Ask the user to rephrase the topic (an English name often works best), then rerun resolve.",
					"no Wikidata entity with articles in %s matches %q", strings.Join(langs, ","), sub))
			case "needs_choice":
				var list []map[string]any
				for _, c := range alts {
					list = append(list, map[string]any{"qid": c.QID, "label": c.Label, "description": c.Description, "langs_with_article": sortedKeys(c.Sitelinks)})
				}
				out.Print(map[string]any{"status": "needs_choice", "topic": sub, "candidates": list,
					"next_step": fmt.Sprintf("Ask the user which meaning of %q they want (do not guess), then run: %s resolve --qid <QID>[,<QID of other basket topics>] --langs %s", sub, e.bin, strings.Join(langs, ","))})
				return
			}
			qids = append(qids, chosen.QID)
			auto = append(auto, alts...)
		}
		qids = uniq(qids)
		if len(qids) == 0 {
			out.Fail(out.Errf("TOPIC_NOT_FOUND", "Ask the user to rephrase the topics (English names often work best), then rerun resolve.",
				"none of the topics has articles in %s", strings.Join(langs, ",")))
		}
	}
	items, err := w.Entities(qids, langs, langs[0])
	if err != nil {
		out.Fail(err)
	}
	if len(items) == 0 {
		out.Fail(out.Errf("TOPIC_NOT_FOUND", "Check the QID.", "no Wikidata entity %v", qids))
	}
	s := &spec.Spec{Topic: *topic, Langs: langs, Articles: map[string][]spec.Article{}, Missing: map[string][]string{},
		End: *end, WindowMonths: *months, Access: *access,
		Criteria: spec.Criteria{MinGrowthPct: *minGrowth, MinMonthlyViews: *minViews}, ResolvedAt: e.now.UTC().Format(time.RFC3339)}
	if s.Topic == "" {
		s.Topic = items[0].Label
	}
	articles := map[string]any{}
	var found []string
	for _, it := range items {
		s.Items = append(s.Items, spec.Item{QID: it.QID, Label: it.Label, Description: it.Description})
	}
	for _, l := range langs {
		var titles []string
		for _, it := range items {
			t, ok := it.Sitelinks[l]
			if !ok {
				s.Missing[l] = append(s.Missing[l], it.QID)
				continue
			}
			rd, err := w.Redirects(l, t, *maxRedirects)
			if err != nil {
				out.Fail(err)
			}
			s.Articles[l] = append(s.Articles[l], spec.Article{QID: it.QID, Title: t, Redirects: rd})
			titles = append(titles, fmt.Sprintf("%s (+%d redirects)", t, len(rd)))
		}
		if len(titles) > 0 {
			found = append(found, l)
			articles[l] = titles
		} else {
			delete(s.Articles, l)
		}
	}
	if len(s.Missing) == 0 {
		s.Missing = nil
	}
	if len(found) == 0 {
		out.Fail(out.Errf("NO_ARTICLES", "Try other languages or a broader topic.", "the topic has no article in any of %v", langs))
	}
	s.Langs = found
	if err := s.Validate(e.now); err != nil {
		out.Fail(err)
	}
	id := s.RunID()
	dir := e.runDir(id)
	_ = os.MkdirAll(dir, 0o755)
	specPath := filepath.Join(dir, "spec.json")
	if err := out.WriteJSON(specPath, s); err != nil {
		out.Fail(err)
	}
	res := map[string]any{"status": "ok", "run_id": id, "topic": s.Topic, "items": s.Items, "articles": articles,
		"window_months": s.WindowMonths, "end": s.End, "spec": e.rel(specPath),
		"next_step": fmt.Sprintf("%s analyze --spec %s", e.bin, e.rel(specPath))}
	if s.Missing != nil {
		res["missing"] = s.Missing
		res["missing_note"] = "No article in these languages: say so in the answer (possible unfilled niche); they are excluded from the analysis."
	}
	if len(skipped) > 0 {
		res["skipped_topics"] = skipped
		res["skipped_note"] = "No article in the requested languages for these basket topics; mention it to the user."
	}
	if len(auto) > 0 {
		var alt []string
		for _, c := range auto[:min(len(auto), 3)] {
			alt = append(alt, fmt.Sprintf("%s: %s (%s)", c.QID, c.Label, c.Description))
		}
		res["other_meanings"] = alt
		res["note"] = "Chosen as the clearly most notable meaning. If the user meant another one, rerun resolve with --qid."
	}
	out.Print(res)
}

// exactFirst returns only exact label/alias matches when there are any
// (a fuzzy "IELTS" → "Yeltsin" match must not compete), else all hits.
func exactFirst(hits []wiki.Hit) []string {
	var exact, all []string
	for _, h := range hits {
		all = append(all, h.QID)
		if h.Exact {
			exact = append(exact, h.QID)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return all
}

// choose picks the entity to analyse. It never guesses between comparable
// meanings: if two candidates both have articles in the requested languages
// and similar notability, the user must choose.
func choose(cands []wiki.Candidate, langs []string) (wiki.Candidate, []wiki.Candidate, string) {
	var full, partial []wiki.Candidate
	for _, c := range cands {
		if c.Disambig || len(c.Sitelinks) == 0 {
			continue
		}
		if len(c.Sitelinks) == len(langs) {
			full = append(full, c)
		} else {
			partial = append(partial, c)
		}
	}
	pool := full
	if len(pool) == 0 {
		pool = partial
	}
	if len(pool) == 0 {
		return wiki.Candidate{}, nil, "not_found"
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].TotalWikis > pool[j].TotalWikis })
	if len(pool) == 1 || pool[0].TotalWikis >= 3*max(pool[1].TotalWikis, 1) {
		var others []wiki.Candidate
		for _, c := range pool[1:min(len(pool), 4)] {
			others = append(others, c)
		}
		return pool[0], others, "ok"
	}
	return wiki.Candidate{}, pool[:min(len(pool), 5)], "needs_choice"
}

// ---------------- analyze ----------------

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func analyze(e env, args []string) {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	specPath := fs.String("spec", "", "path to spec.json from resolve")
	from := fs.String("from", "", "run_id of a previous run to derive from")
	var sets multi
	fs.Var(&sets, "set", "override key=value (repeatable): window_months, end, access, langs, min_growth_pct, min_monthly_views")
	parse(fs, args)

	var s spec.Spec
	switch {
	case *from != "":
		p := filepath.Join(e.runDir(*from), "spec.json")
		if err := out.ReadJSON(p, &s); err != nil {
			out.Fail(out.Errf("RUN_NOT_FOUND", "Use a run_id printed by resolve or analyze.", "cannot read %s", p))
		}
		for _, kv := range sets {
			if err := s.Set(kv); err != nil {
				if oe, ok := err.(*out.Error); ok && strings.HasPrefix(oe.NextStep, "Run: resolve") {
					oe.NextStep = "Run: " + e.bin + strings.TrimPrefix(oe.NextStep, "Run:")
				}
				out.Fail(err)
			}
		}
		s.ParentRunID = *from
		s.ResolvedAt = ""
	case *specPath != "":
		*specPath = e.abs(*specPath)
		if err := out.ReadJSON(*specPath, &s); err != nil {
			out.Fail(out.Errf("SPEC_NOT_FOUND", "Use the spec path printed by resolve.", "cannot read %s: %v", *specPath, err))
		}
		if len(sets) > 0 {
			out.Fail(out.Errf("BAD_ARGS", "Use --set only together with --from <run_id>.", "--set requires --from"))
		}
	default:
		out.Fail(out.Errf("BAD_ARGS", fmt.Sprintf("%s analyze --spec <path> | --from <run_id> --set key=value", e.bin), "--spec or --from is required"))
	}
	if err := s.Validate(e.now); err != nil {
		out.Fail(err)
	}
	id := s.RunID()
	dir := e.runDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		out.Fail(err)
	}
	_ = out.WriteJSON(filepath.Join(dir, "spec.json"), s)

	w, h := e.client()
	data, err := analysis.Fetch(w, &s)
	if err != nil {
		out.Fail(err)
	}
	r, err := analysis.Compute(&s, data)
	if err != nil {
		out.Fail(out.Errf("INSUFFICIENT_DATA", "Try a shorter window (--set window_months=24) or a later end month.", "%v", err))
	}
	resPath := filepath.Join(dir, "result.json")
	if err := out.WriteJSON(resPath, r); err != nil {
		out.Fail(err)
	}
	_ = out.WriteJSON(filepath.Join(dir, "data.json"), data)
	chartPath := filepath.Join(dir, "chart.svg")
	L := "uk"
	_ = os.WriteFile(chartPath, []byte(report.ChartSVG(r, r.Topic+" — "+report.ChartTitle(L), report.BandLabel(L))), 0o644)

	var langs []map[string]any
	for _, l := range r.Langs {
		var reasons, warnings []string
		maxReasons := 3
		if len(r.Langs) > 3 {
			maxReasons = 1 // keep stdout small; `explain` has the details
		}
		for i, rr := range l.Reasons {
			if i < maxReasons {
				reasons = append(reasons, rr.Text)
			}
		}
		for _, wr := range l.Warnings {
			if len(r.Langs) <= 3 {
				warnings = append(warnings, wr.Text)
			} else {
				warnings = append(warnings, wr.Code)
			}
		}
		m := map[string]any{"lang": l.Lang, "monthly_views_median": math.Round(l.MonthlyViewsMed),
			"share_trend_pct_per_year": r1(l.ShareTrend.Est), "interval_90pct": []float64{r1(l.ShareTrend.CI90[0]), r1(l.ShareTrend.CI90[1])},
			"verdict": l.Verdict, "trust": l.Trust, "reasons": reasons, "meets_criteria": l.MeetsCriteria}
		if l.ShareYoYPct != nil {
			m["share_yoy_pct"] = r1(*l.ShareYoYPct)
		}
		if len(r.Langs) <= 3 {
			m["titles"] = l.Titles
		}
		if len(warnings) > 0 {
			m["warnings"] = warnings
		}
		langs = append(langs, m)
	}
	resOut := map[string]any{"status": "ok", "run_id": id, "topic": r.Topic, "period": r.Window, "langs": langs, "ranking": r.Ranking,
		"requests": map[string]int{"network": h.Requests, "cache_hits": h.CacheHits},
		"files":    map[string]string{"result": e.rel(resPath), "chart": e.rel(chartPath)},
		"placeholders": "Numbers in the report text must be placeholders: {{<lang>.trend}}, {{<lang>.trend_ci}}, {{<lang>.share_yoy}}, {{<lang>.monthly_views}}, " +
			"{{<lang>.trust}}, {{<lang>.verdict}}, {{<lang>.spike_month}}, {{<lang>.edition_trend}}, {{rank.1}}, {{period.start}}, {{period.end}}, {{period.months}}, {{topic}}",
		"next_step": fmt.Sprintf("Answer the user from these results. For a shareable one-page PDF: write a narrative file (first line = headline, then 2-4 short paragraphs, numbers ONLY as placeholders) to %s, then run: %s report --run %s --narrative %s",
			e.rel(filepath.Join(dir, "narrative.md")), e.bin, id, e.rel(filepath.Join(dir, "narrative.md")))}
	if r.ParentRunID != "" {
		resOut["parent_run_id"] = r.ParentRunID
	}
	if len(r.Missing) > 0 {
		resOut["missing"] = r.Missing
	}
	out.Print(resOut)
}

func r1(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10) / 10
}

// ---------------- report ----------------

func loadResult(e env, id string) *analysis.Result {
	var r analysis.Result
	p := filepath.Join(e.runDir(id), "result.json")
	if err := out.ReadJSON(p, &r); err != nil {
		out.Fail(out.Errf("RUN_NOT_FOUND", "Run analyze first and use the run_id it printed.", "cannot read %s", p))
	}
	return &r
}

func reportCmd(e env, args []string) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	run := fs.String("run", "", "run_id from analyze")
	narr := fs.String("narrative", "", "path to the narrative text (first line = headline)")
	text := fs.String("text", "", "narrative text inline (alternative to --narrative)")
	lang := fs.String("lang", "uk", "report label language: uk|en")
	parse(fs, args)
	if *run == "" || (*narr == "" && *text == "") {
		out.Fail(out.Errf("BAD_ARGS", fmt.Sprintf("%s report --run <run_id> --narrative <file>", e.bin), "--run and --narrative (or --text) are required"))
	}
	r := loadResult(e, *run)
	body := *text
	if *narr != "" {
		*narr = e.abs(*narr)
		b, err := os.ReadFile(*narr)
		if err != nil {
			out.Fail(out.Errf("BAD_ARGS", "Write the narrative file first (see next_step of analyze).", "cannot read %s", *narr))
		}
		body = string(b)
	}
	retry := fmt.Sprintf("Fix the narrative file and rerun: %s report --run %s --narrative %s", e.bin, *run, *narr)
	n, err := report.Render(body, report.Localize(r.Placeholder, *lang), retry)
	if err != nil {
		out.Fail(err)
	}
	dir := e.runDir(*run)
	pdfPath := filepath.Join(dir, "report.pdf")
	if err := report.Build(r, n, report.Options{Lang: *lang, Path: pdfPath, Now: e.now, Next: retry}); err != nil {
		out.Fail(err)
	}
	chartPath := filepath.Join(dir, "chart.svg")
	_ = os.WriteFile(chartPath, []byte(report.ChartSVG(r, r.Topic+" — "+report.ChartTitle(*lang), report.BandLabel(*lang))), 0o644)
	out.Print(map[string]any{"status": "ok", "run_id": *run, "pdf": pdfPath, "chart": chartPath, "pages": 1,
		"headline": n.Headline, "body": n.Body,
		"next_step": fmt.Sprintf("Give the user the PDF path and a short summary. Follow-up questions: %s analyze --from %s --set key=value (e.g. window_months=24, access=desktop, min_growth_pct=15)", e.bin, *run)})
}

// ---------------- explain ----------------

func explain(e env, args []string) {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	run := fs.String("run", "", "run_id")
	lang := fs.String("lang", "", "only this language")
	parse(fs, args)
	if *run == "" {
		out.Fail(out.Errf("BAD_ARGS", e.bin+" explain --run <run_id> [--lang uk]", "--run is required"))
	}
	r := loadResult(e, *run)
	var langs []map[string]any
	for _, l := range r.Langs {
		if *lang != "" && l.Lang != *lang {
			continue
		}
		var checks []string
		for _, c := range l.Checks {
			st := "n/a"
			if c.Pass != nil {
				st = map[bool]string{true: "pass", false: "FAIL"}[*c.Pass]
			}
			checks = append(checks, fmt.Sprintf("%s %s: %s — %s", c.ID, c.Name, st, c.Detail))
		}
		langs = append(langs, map[string]any{"lang": l.Lang, "verdict": l.Verdict, "trust": l.Trust, "checks": checks,
			"spike_months": l.SpikeMonths, "edition_views_trend_pct_per_year": r1(l.EditionTrendPct)})
	}
	out.Print(map[string]any{"status": "ok", "run_id": *run, "langs": langs,
		"rules":     "high: C1-C5 pass and (C6 or C7). medium: C1 and C4 pass, one other check fails. low: C1 or C4 fails, or two others fail.",
		"next_step": "Explain the failing checks to the user in plain words."})
}

func uniq(s []string) []string {
	var r []string
	seen := map[string]bool{}
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			r = append(r, x)
		}
	}
	return r
}

func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}
