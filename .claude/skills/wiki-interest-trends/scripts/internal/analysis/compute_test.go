package analysis

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"wikitrend/internal/spec"
)

// synth builds a synthetic language edition with a known answer.
type synth struct {
	perDay       float64 // topic views per day at the start
	shareGrowth  float64 // true growth of the topic's share, per year (0.2 = +20%)
	editionTrend float64 // growth of the whole edition per year (-0.08 = −8%)
	monthlyNoise float64 // sd of AR(1) month-level multiplicative noise
	season       float64 // amplitude of the yearly cycle
	spikes       map[int]float64
	seed         int64
}

func (g synth) build(s *spec.Spec) *LangData {
	rng := rand.New(rand.NewSource(g.seed))
	from, toM := Period(s)
	to := toM.AddDate(0, 1, -1)
	ld := &LangData{PerItem: map[string][]float64{}}
	for m := from; !m.After(toM); m = m.AddDate(0, 1, 0) {
		ld.Months = append(ld.Months, m.Format("2006-01"))
	}
	monthEff := map[string]float64{}
	e := 0.0
	for _, m := range ld.Months {
		e = 0.5*e + g.monthlyNoise*rng.NormFloat64()
		monthEff[m] = e
	}
	aggDaily := map[string]float64{}
	i := 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		t := d.Sub(from).Hours() / 24 / 365
		m := d.Format("2006-01")
		edition := 5e6 * math.Pow(1+g.editionTrend, t)
		shareMult := math.Pow(1+g.shareGrowth, t) * (1 + g.season*math.Sin(2*math.Pi*float64(d.YearDay())/365)) * math.Exp(monthEff[m])
		lambda := g.perDay * shareMult * edition / 5e6
		v := math.Max(0, math.Round(lambda+math.Sqrt(lambda)*rng.NormFloat64()))
		if k, ok := g.spikes[i]; ok {
			v += k * g.perDay
		}
		ld.Days = append(ld.Days, d.Format("2006-01-02"))
		ld.Total = append(ld.Total, v)
		aggDaily[m] += edition
		i++
	}
	for _, m := range ld.Months {
		ld.Agg = append(ld.Agg, aggDaily[m])
	}
	ld.PerItem["Q1"] = ld.Total
	return ld
}

func testSpec() *spec.Spec {
	s := &spec.Spec{Topic: "t", Items: []spec.Item{{QID: "Q1"}}, Langs: []string{"xx"},
		Articles: map[string][]spec.Article{"xx": {{QID: "Q1", Title: "T"}}}}
	if err := s.Validate(time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)); err != nil {
		panic(err)
	}
	return s
}

func run(t *testing.T, g synth) *LangResult { return runW(t, g, spec.DefaultWindow) }

func runW(t *testing.T, g synth, window int) *LangResult {
	t.Helper()
	s := testSpec()
	s.WindowMonths = window
	r, err := Compute(s, map[string]*LangData{"xx": g.build(s)})
	if err != nil {
		t.Fatal(err)
	}
	return r.Langs[0]
}

func TestRecoversGrowthDespiteEditionDecline(t *testing.T) {
	l := run(t, synth{perDay: 1000, shareGrowth: 0.20, editionTrend: -0.08, monthlyNoise: 0.03, season: 0.1, seed: 1})
	t.Logf("trend %+.1f%% CI %v, raw views YoY %.1f%%, verdict %s, trust %s", l.ShareTrend.Est, l.ShareTrend.CI90, *l.ViewsYoYPct, l.Verdict, l.Trust)
	if math.Abs(l.ShareTrend.Est-20) > 5 {
		t.Errorf("share trend %.1f, want ≈ +20", l.ShareTrend.Est)
	}
	if l.Verdict != "growing" || l.Trust != "high" {
		t.Errorf("verdict/trust = %s/%s, want growing/high", l.Verdict, l.Trust)
	}
	if l.EditionTrendPct > -5 {
		t.Errorf("edition trend %.1f, want ≈ −8", l.EditionTrendPct)
	}
}

func TestFlatWithSpikeIsNotHigh(t *testing.T) {
	// Flat interest, one viral day worth 60 normal days near the end.
	l := run(t, synth{perDay: 800, monthlyNoise: 0.03, season: 0.05, seed: 2, spikes: map[int]float64{1080: 60}})
	t.Logf("trend %+.1f (raw %+.1f), top3 %.2f, verdict %s, trust %s, reasons %v", l.ShareTrend.Est, l.RawShareTrendPct, l.Top3DaysShare, l.Verdict, l.Trust, l.Reasons)
	if l.Trust == "high" {
		t.Fatal("a single spike must not produce high trust")
	}
	if l.Verdict == "growing" && l.Trust != "low" {
		t.Fatalf("flat series with spike called growing with %s trust", l.Trust)
	}
}

func TestLowVolumeIsLow(t *testing.T) {
	l := run(t, synth{perDay: 5, shareGrowth: 0.3, monthlyNoise: 0.05, seed: 3})
	if l.Trust != "low" || !hasReason(l, "low_volume") {
		t.Fatalf("trust %s reasons %v", l.Trust, l.Reasons)
	}
}

func TestDecline(t *testing.T) {
	l := run(t, synth{perDay: 2000, shareGrowth: -0.25, monthlyNoise: 0.03, seed: 4})
	if l.Verdict != "declining" {
		t.Fatalf("verdict %s, trend %.1f", l.Verdict, l.ShareTrend.Est)
	}
}

func TestPlaceholdersAndRanking(t *testing.T) {
	s := testSpec()
	s.Langs = []string{"aa", "bb"}
	s.Articles = map[string][]spec.Article{"aa": {{QID: "Q1", Title: "A"}}, "bb": {{QID: "Q1", Title: "B"}}}
	data := map[string]*LangData{
		"aa": synth{perDay: 1500, shareGrowth: -0.2, monthlyNoise: 0.03, seed: 5}.build(s),
		"bb": synth{perDay: 1500, shareGrowth: 0.3, monthlyNoise: 0.03, seed: 6}.build(s),
	}
	r, err := Compute(s, data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ranking[0] != "bb" {
		t.Fatalf("ranking %v, growing language must be first", r.Ranking)
	}
	for _, k := range []string{"aa.trend", "bb.trust", "rank.1", "period.start", "bb.share_yoy"} {
		if r.Placeholder[k] == "" {
			t.Errorf("placeholder %s missing", k)
		}
	}
	if r.Window.Months != 36 {
		t.Errorf("window months %d", r.Window.Months)
	}
}

func hasReason(l *LangResult, code string) bool {
	for _, r := range l.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}
