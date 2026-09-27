// Package analysis turns raw pageviews into the metrics, the trust level and
// its reasons. Compute is pure (no network) and tested on synthetic series.
package analysis

import (
	"fmt"
	"math"
	"sort"

	"wikitrend/internal/spec"
	"wikitrend/internal/stats"
)

// Thresholds of the trust checks. They are hypotheses calibrated on synthetic
// series (see calibrate_test.go and references/METHODOLOGY.md).
const (
	HampelHalfWindow = 3    // days on each side
	HampelSigmas     = 3.0  // outlier threshold in robust sigmas
	PValue           = 0.05 // C1
	MinEffectPct     = 10.0 // C2: |trend| per year that is practically meaningful
	SpikeChangeMax   = 0.5  // C3/C6: trend may change at most 50% (relative) …
	ChangeFloorPP    = 5.0  // … or 5 percentage points/yr, whichever is larger (near-zero trends)
	Top3DaysMax      = 0.20 // C3: top-3 days share of last-year views
	MinVolume        = 1000 // C4: median views/month for a reliable series
	HardMinVolume    = 300  // below this the level is always "low"
	CIConf           = 0.90
)

// Bot reclassification by Wikimedia (Diff blog, 2025-10-17): March–August 2025.
var botBreak = [2]string{"2025-03", "2025-08"}

// Check is one trust check. Pass is nil when the check does not apply.
type Check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Pass   *bool  `json:"pass"`
	Detail string `json:"detail"`
}

// Reason explains the trust level. Code is stable (localised in the report).
type Reason struct {
	Code   string            `json:"code"`
	Text   string            `json:"text"`
	Params map[string]string `json:"params,omitempty"`
}

// Trend is a growth rate in % per year with a 90% interval.
type Trend struct {
	Est  float64    `json:"est"`
	CI90 [2]float64 `json:"ci90"`
}

// LangResult is the full result for one language.
type LangResult struct {
	Lang              string    `json:"lang"`
	Titles            []string  `json:"titles"`
	MonthlyViewsMed   float64   `json:"monthly_views_median"`
	ShareYoYPct       *float64  `json:"share_yoy_pct"`
	ViewsYoYPct       *float64  `json:"views_yoy_pct"`
	ShareTrend        Trend     `json:"share_trend_pct_per_year"`
	RawShareTrendPct  float64   `json:"raw_share_trend_pct_per_year"`
	EditionTrendPct   float64   `json:"edition_views_trend_pct_per_year"`
	MK                stats.MK  `json:"mann_kendall"`
	ACFactor          float64   `json:"autocorr_var_factor"`
	Top3DaysShare     float64   `json:"top3_days_share"`
	SpikeMonths       []string  `json:"spike_months"`
	Verdict           string    `json:"verdict"` // growing | declining | no_clear_trend
	Trust             string    `json:"trust"`   // high | medium | low
	Reasons           []Reason  `json:"reasons"`
	Warnings          []Reason  `json:"warnings,omitempty"`
	Checks            []Check   `json:"checks"`
	MeetsCriteria     bool      `json:"meets_criteria"`
	Months            []string  `json:"months"`        // analysis window
	ShareRaw          []float64 `json:"share_raw"`     // per million views
	ShareClean        []float64 `json:"share_clean"`   // per million views, spikes removed
	ViewsRaw          []float64 `json:"views_raw"`     // topic views per month
	EditionViews      []float64 `json:"edition_views"` // edition views per month
	TrendLine         []float64 `json:"trend_line"`    // fitted share, per million
	crossesBotBreakAt bool
}

// Result is what `analyze` writes to result.json.
type Result struct {
	RunID       string               `json:"run_id"`
	ParentRunID string               `json:"parent_run_id,omitempty"`
	Topic       string               `json:"topic"`
	Items       []spec.Item          `json:"items"`
	Window      Window               `json:"period"`
	Access      string               `json:"access"`
	Criteria    spec.Criteria        `json:"criteria"`
	Langs       []*LangResult        `json:"langs"`
	Missing     map[string][]string  `json:"missing,omitempty"`
	Ranking     []string             `json:"ranking"`
	Placeholder map[string]string    `json:"placeholders"`
	Data        map[string]*LangData `json:"-"`
	Extra       map[string]any       `json:"extra,omitempty"`
}

// Window is the analysis window.
type Window struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Months int    `json:"months"`
}

func ptr[T any](v T) *T { return &v }

// Compute runs the whole methodology for every language of s.
func Compute(s *spec.Spec, data map[string]*LangData) (*Result, error) {
	r := &Result{RunID: s.RunID(), ParentRunID: s.ParentRunID, Topic: s.Topic, Items: s.Items,
		Access: s.Access, Criteria: s.Criteria, Missing: s.Missing, Data: data}
	for _, lang := range s.Langs {
		ld, ok := data[lang]
		if !ok {
			continue
		}
		lr, err := computeLang(s, lang, ld)
		if err != nil {
			return nil, err
		}
		r.Langs = append(r.Langs, lr)
		r.Window = Window{Start: lr.Months[0], End: lr.Months[len(lr.Months)-1], Months: len(lr.Months)}
	}
	r.Ranking = rank(r.Langs)
	r.Placeholder = placeholders(r)
	return r, nil
}

type monthly struct {
	months          []string
	raw, clean, agg []float64
	spikeMonths     []string
	dailyRaw        []float64
	days            []string
}

func toMonthly(ld *LangData, daily []float64) monthly {
	clean, _ := stats.Hampel(daily, HampelHalfWindow, HampelSigmas)
	idx := map[string]int{}
	for i, m := range ld.Months {
		idx[m] = i
	}
	m := monthly{months: ld.Months, agg: ld.Agg, dailyRaw: daily, days: ld.Days,
		raw: make([]float64, len(ld.Months)), clean: make([]float64, len(ld.Months))}
	for i, d := range ld.Days {
		k := idx[d[:7]]
		m.raw[k] += daily[i]
		m.clean[k] += clean[i]
	}
	for i := range m.months {
		if m.raw[i] > 0 && (m.raw[i]-m.clean[i])/m.raw[i] > 0.10 {
			m.spikeMonths = append(m.spikeMonths, m.months[i])
		}
	}
	return m
}

func share(views, agg []float64) []float64 {
	s := make([]float64, len(views))
	for i := range views {
		if agg[i] > 0 {
			s[i] = views[i] / agg[i] * 1e6
		}
	}
	return s
}

// fit returns the trend in %/year of a share series via Theil–Sen on log(share),
// skipping zero months, plus the 90% Sen interval widened for autocorrelation.
type fitRes struct {
	est, lo, hi float64
	mk          stats.MK
	acf         float64
	line        []float64
	n           int
}

func pctYear(slope float64) float64 { return (math.Exp(12*slope) - 1) * 100 }

func fitTrend(sh []float64, skip func(i int) bool, seasonal bool) fitRes {
	var x, y []float64
	for i, v := range sh {
		if v > 0 && (skip == nil || !skip(i)) {
			x = append(x, float64(i))
			y = append(y, math.Log(v))
		}
	}
	if len(x) < 6 {
		return fitRes{est: math.NaN(), lo: math.NaN(), hi: math.NaN(), mk: stats.MK{P: 1}, acf: 1}
	}
	slope, icpt := stats.TheilSen(x, y)
	res := make([]float64, len(x))
	for i := range x {
		res[i] = y[i] - (icpt + slope*x[i])
	}
	// Autocorrelation is measured after removing the calendar-month means:
	// otherwise the yearly cycle itself looks like autocorrelation and the
	// interval explodes (TestRecoversGrowthDespiteEditionDecline caught this).
	if len(x) >= 24 {
		var sums, cnt [12]float64
		for i := range x {
			k := int(x[i]) % 12
			sums[k] += res[i]
			cnt[k]++
		}
		for i := range x {
			if k := int(x[i]) % 12; cnt[k] > 1 {
				res[i] -= sums[k] / cnt[k]
			}
		}
	}
	acf := stats.ACFactor(res)
	lo, hi := stats.SenCI(x, y, CIConf, acf)
	var mk stats.MK
	if seasonal && len(sh) >= 24 && skip == nil {
		// Seasonal MK compares January with January etc. Neighbouring seasons
		// are still correlated (a good March usually follows a good February),
		// so the same deseasonalised autocorrelation factor is applied.
		mk = stats.SeasonalMannKendall(sh, 12).WithVarFactor(acf)
	} else {
		mk = stats.MannKendall(y).WithVarFactor(acf)
	}
	line := make([]float64, len(sh))
	for i := range sh {
		line[i] = math.Exp(icpt + slope*float64(i))
	}
	return fitRes{est: pctYear(slope), lo: pctYear(lo), hi: pctYear(hi), mk: mk, acf: acf, line: line, n: len(x)}
}

// similar reports whether an alternative trend estimate tells the same story
// as the main one: close in value, and not of the opposite sign unless both are tiny.
func similar(alt, main float64) bool {
	if math.IsNaN(alt) || math.IsNaN(main) {
		return false
	}
	diff := math.Abs(alt - main)
	if diff >= math.Max(SpikeChangeMax*math.Abs(main), ChangeFloorPP) {
		return false
	}
	return sign(alt) == sign(main) || math.Abs(alt) < ChangeFloorPP && math.Abs(main) < ChangeFloorPP
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func sum(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s
}

func computeLang(s *spec.Spec, lang string, ld *LangData) (*LangResult, error) {
	W := s.WindowMonths
	n := len(ld.Months)
	if n < W {
		return nil, fmt.Errorf("%s: only %d months of data for a %d-month window", lang, n, W)
	}
	mo := toMonthly(ld, ld.Total)
	shRaw, shClean := share(mo.raw, mo.agg), share(mo.clean, mo.agg)
	w0 := n - W // window start index
	win := func(v []float64) []float64 { return append([]float64(nil), v[w0:]...) }

	lr := &LangResult{Lang: lang, Months: append([]string(nil), mo.months[w0:]...), ShareRaw: win(shRaw), ShareClean: win(shClean),
		ViewsRaw: win(mo.raw), EditionViews: win(mo.agg)}
	for _, a := range s.Articles[lang] {
		lr.Titles = append(lr.Titles, a.Title)
	}
	lr.MonthlyViewsMed = stats.Median(lr.ViewsRaw)

	// YoY on 12-month sums (needs 24 months of history).
	if n >= 24 {
		s1, s0 := sum(mo.raw[n-12:]), sum(mo.raw[n-24:n-12])
		a1, a0 := sum(mo.agg[n-12:]), sum(mo.agg[n-24:n-12])
		if s0 > 0 && a0 > 0 && a1 > 0 {
			lr.ShareYoYPct = ptr(((s1/a1)/(s0/a0) - 1) * 100)
			lr.ViewsYoYPct = ptr((s1/s0 - 1) * 100)
		}
	}

	main := fitTrend(lr.ShareClean, nil, true)
	raw := fitTrend(lr.ShareRaw, nil, true)
	lr.ShareTrend = Trend{Est: main.est, CI90: [2]float64{main.lo, main.hi}}
	lr.RawShareTrendPct = raw.est
	lr.MK, lr.ACFactor, lr.TrendLine = main.mk, main.acf, main.line
	lr.EditionTrendPct = fitTrend(lr.EditionViews, nil, false).est

	// Spikes: top-3 days in the last 12 months.
	last12 := mo.dailyRaw[len(mo.dailyRaw)-daysInLastMonths(mo.days, 12):]
	if tot := sum(last12); tot > 0 {
		sorted := append([]float64(nil), last12...)
		sort.Sort(sort.Reverse(sort.Float64Slice(sorted)))
		lr.Top3DaysShare = sum(sorted[:min(3, len(sorted))]) / tot
	}
	for _, m := range mo.spikeMonths {
		if m >= lr.Months[0] {
			lr.SpikeMonths = append(lr.SpikeMonths, m)
		}
	}

	// ---- checks C1..C7 ----
	dir := sign(main.est)
	var checks []Check
	add := func(id, name string, pass *bool, detail string) {
		checks = append(checks, Check{id, name, pass, detail})
	}

	c1 := !math.IsNaN(main.est) && main.mk.P < PValue && sign(main.mk.S) == dir && dir != 0
	add("C1", "direction", &c1, fmt.Sprintf("Mann-Kendall p=%.3f (autocorrelation factor %.2f)", main.mk.P, main.acf))

	c2 := math.Abs(main.est) >= MinEffectPct && sign(main.lo) == sign(main.hi) && sign(main.lo) == dir
	add("C2", "magnitude", &c2, fmt.Sprintf("trend %+.1f%%/yr, 90%% CI [%+.1f, %+.1f]", main.est, main.lo, main.hi))

	c3 := similar(raw.est, main.est) && lr.Top3DaysShare < Top3DaysMax
	add("C3", "spikes", &c3, fmt.Sprintf("trend with spikes %+.1f%%/yr vs without %+.1f%%/yr; top-3 days = %.0f%% of last-year views", raw.est, main.est, lr.Top3DaysShare*100))

	c4 := lr.MonthlyViewsMed >= MinVolume
	add("C4", "volume", &c4, fmt.Sprintf("median %.0f views/month (threshold %d)", lr.MonthlyViewsMed, MinVolume))

	c5 := true
	var alt []string
	// Same window length ending 3 months earlier.
	if n >= W+3 {
		f := fitTrend(shClean[n-W-3:n-3], nil, true)
		alt = append(alt, fmt.Sprintf("ending 3 months earlier %+.1f%%", f.est))
		c5 = c5 && sign(f.est) == dir
	}
	// A different window length: 36 months (or 24 if the window is already ≥ 36).
	otherW := 36
	if W >= 36 {
		otherW = 24
	}
	if n >= otherW {
		f := fitTrend(shClean[n-otherW:], nil, true)
		alt = append(alt, fmt.Sprintf("%d-month window %+.1f%%", otherW, f.est))
		c5 = c5 && sign(f.est) == dir
	}
	add("C5", "window sensitivity", &c5, joinOr(alt, "not enough history"))

	var c6 *bool
	skipBreak := func(i int) bool { return lr.Months[i] >= botBreak[0] && lr.Months[i] <= botBreak[1] }
	for _, m := range lr.Months {
		if m >= botBreak[0] && m <= botBreak[1] {
			lr.crossesBotBreakAt = true
		}
	}
	if lr.crossesBotBreakAt {
		f := fitTrend(lr.ShareClean, skipBreak, false)
		ok := similar(f.est, main.est)
		c6 = &ok
		add("C6", "2025 bot reclassification", c6, fmt.Sprintf("without Mar–Aug 2025: %+.1f%%/yr", f.est))
	} else {
		add("C6", "2025 bot reclassification", nil, "window does not cross Mar–Aug 2025")
	}

	var c7 *bool
	if len(ld.PerItem) > 1 {
		same, total := 0, 0
		var parts []string
		for _, a := range s.Articles[lang] {
			im := toMonthly(ld, ld.PerItem[a.QID])
			f := fitTrend(share(im.clean, im.agg)[w0:], nil, false)
			if math.IsNaN(f.est) {
				continue
			}
			total++
			if sign(f.est) == dir {
				same++
			}
			parts = append(parts, fmt.Sprintf("%s %+.1f%%", a.Title, f.est))
		}
		if total > 1 {
			ok := float64(same) >= 2.0/3.0*float64(total)
			c7 = &ok
			add("C7", "basket consistency", c7, fmt.Sprintf("%d/%d articles move the same way: %s", same, total, joinOr(parts, "")))
		}
	}
	if c7 == nil {
		add("C7", "basket consistency", nil, "single article")
	}
	lr.Checks = checks

	// ---- verdict & level ----
	switch {
	case c1 && dir > 0:
		lr.Verdict = "growing"
	case c1 && dir < 0:
		lr.Verdict = "declining"
	default:
		lr.Verdict = "no_clear_trend"
	}
	breakOK := (c6 == nil || *c6) || (c7 != nil && *c7)
	fails := 0
	for _, ok := range []bool{c2, c3, c5, breakOK} {
		if !ok {
			fails++
		}
	}
	switch {
	case !c1 || !c4 || lr.MonthlyViewsMed < HardMinVolume || fails >= 2:
		lr.Trust = "low"
	case fails == 0:
		lr.Trust = "high"
	default:
		lr.Trust = "medium"
	}

	lr.Reasons, lr.Warnings = explain(lr, c1, c2, c3, c4, c5, c6, c7)
	lr.MeetsCriteria = lr.Verdict == "growing" && main.est >= s.Criteria.MinGrowthPct &&
		lr.MonthlyViewsMed >= s.Criteria.MinMonthlyViews
	return lr, nil
}

func explain(lr *LangResult, c1, c2, c3, c4, c5 bool, c6, c7 *bool) (reasons, warnings []Reason) {
	f1 := func(v float64) string { return fmt.Sprintf("%.0f", v) }
	r := func(code, text string, kv ...string) Reason {
		p := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			p[kv[i]] = kv[i+1]
		}
		for k, v := range p {
			text = replaceAll(text, "{"+k+"}", v)
		}
		return Reason{Code: code, Text: text, Params: p}
	}
	if !c4 {
		reasons = append(reasons, r("low_volume", "low volume: median {views} views/month (reliable from {min})", "views", f1(lr.MonthlyViewsMed), "min", f1(MinVolume)))
	}
	if c1 {
		reasons = append(reasons, r("direction_clear", "direction is statistically clear (Mann-Kendall p={p})", "p", fmt.Sprintf("%.3f", lr.MK.P)))
	} else {
		reasons = append(reasons, r("direction_unclear", "no statistically clear direction; the change is within noise (Mann-Kendall p={p})", "p", fmt.Sprintf("%.2f", lr.MK.P)))
	}
	if c1 && !c2 {
		reasons = append(reasons, r("effect_small", "the change is small or its 90% interval includes zero (threshold {min}%/yr)", "min", f1(MinEffectPct)))
	}
	if !c3 {
		if lr.Top3DaysShare >= Top3DaysMax {
			reasons = append(reasons, r("spike_days", "a few days dominate: the top-3 days are {share}% of last year's views", "share", f1(lr.Top3DaysShare*100)))
		} else {
			m := "—"
			if len(lr.SpikeMonths) > 0 {
				m = lr.SpikeMonths[len(lr.SpikeMonths)-1]
			}
			reasons = append(reasons, r("spikes", "one-off spikes affect the result (latest: {month}); trend without them {clean}%/yr vs {raw}%/yr with them",
				"month", m, "clean", fmt.Sprintf("%+.1f", lr.ShareTrend.Est), "raw", fmt.Sprintf("%+.1f", lr.RawShareTrendPct)))
		}
	}
	if !c5 {
		reasons = append(reasons, r("window_sensitive", "the direction changes when the time window is shifted or lengthened"))
	}
	if c6 != nil && !*c6 {
		reasons = append(reasons, r("bot_break", "the trend depends on Mar–Aug 2025, when Wikimedia reclassified bot traffic"))
	}
	if c7 != nil && !*c7 {
		reasons = append(reasons, r("basket_mixed", "articles in the topic basket move in different directions"))
	}
	if c7 != nil && *c7 {
		reasons = append(reasons, r("basket_consistent", "articles in the topic basket move in the same direction"))
	}
	if c1 && c2 && c3 && c5 {
		reasons = append(reasons, r("robust", "result holds without spikes and for other time windows"))
	}
	if lr.crossesBotBreakAt && (c6 == nil || *c6) {
		warnings = append(warnings, r("crosses_bot_break", "the period includes Wikimedia's 2025 bot reclassification; share-based metrics reduce but do not remove its effect"))
	}
	if len(lr.Months) < 36 {
		warnings = append(warnings, r("short_window", "a {n}-month window often misses moderate growth (about 1 in 3 series growing +15%/yr is confirmed); use window_months=36 for a firmer answer", "n", fmt.Sprint(len(lr.Months))))
	}
	if first := firstNonZero(lr.ViewsRaw); first > 2 {
		warnings = append(warnings, r("article_new", "no views before {month}: the article may be new or renamed", "month", lr.Months[first]))
	}
	return reasons, warnings
}

var trustRank = map[string]int{"high": 3, "medium": 2, "low": 1}
var verdictRank = map[string]int{"growing": 3, "no_clear_trend": 2, "declining": 1}

// rank orders languages: growing first, then by trust, then by trend.
func rank(ls []*LangResult) []string {
	s := append([]*LangResult(nil), ls...)
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if verdictRank[a.Verdict] != verdictRank[b.Verdict] {
			return verdictRank[a.Verdict] > verdictRank[b.Verdict]
		}
		if trustRank[a.Trust] != trustRank[b.Trust] {
			return trustRank[a.Trust] > trustRank[b.Trust]
		}
		return a.ShareTrend.Est > b.ShareTrend.Est
	})
	out := make([]string, len(s))
	for i, l := range s {
		out[i] = l.Lang
	}
	return out
}
