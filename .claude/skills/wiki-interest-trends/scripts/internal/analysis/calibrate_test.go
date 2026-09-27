package analysis

import (
	"fmt"
	"testing"
)

// TestCalibration measures how often each trust level appears on synthetic
// series with a known answer. Targets (references/METHODOLOGY.md):
//   - no real change: "high" in ≤ 5% of series, "growing" in ≤ 10%;
//   - +15%/yr share growth at 1500 views/day: "high" in ≥ 70%.
func TestCalibration(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation")
	}
	const sims = 200
	classes := []struct {
		name string
		g    synth
	}{
		{"null", synth{perDay: 1500, monthlyNoise: 0.08, season: 0.1, editionTrend: -0.08}},
		{"null+spike", synth{perDay: 1500, monthlyNoise: 0.08, season: 0.1, spikes: map[int]float64{1050: 40}}},
		{"+5%/yr", synth{perDay: 1500, shareGrowth: 0.05, monthlyNoise: 0.08, season: 0.1}},
		{"+15%/yr", synth{perDay: 1500, shareGrowth: 0.15, monthlyNoise: 0.08, season: 0.1, editionTrend: -0.08}},
		{"+30%/yr", synth{perDay: 1500, shareGrowth: 0.30, monthlyNoise: 0.08, season: 0.1}},
		{"+30%/yr low volume", synth{perDay: 8, shareGrowth: 0.30, monthlyNoise: 0.08}},
	}
	rates := map[string]map[string]float64{}
	for _, window := range []int{24, 36} {
		for _, c := range classes {
			name := fmt.Sprintf("%s w%d", c.name, window)
			cnt := map[string]int{}
			for i := range sims {
				g := c.g
				g.seed = int64(1000 + i)
				l := runW(t, g, window)
				cnt[l.Trust]++
				cnt[l.Verdict]++
			}
			r := map[string]float64{}
			for k, v := range cnt {
				r[k] = float64(v) / sims
			}
			rates[name] = r
			t.Logf("%-24s high %.2f  medium %.2f  low %.2f | growing %.2f  no_clear %.2f  declining %.2f", name,
				r["high"], r["medium"], r["low"], r["growing"], r["no_clear_trend"], r["declining"])
		}
	}
	check := func(ok bool, msg string, a ...any) {
		if !ok {
			t.Errorf(msg, a...)
		}
	}
	for _, w := range []string{"w24", "w36"} {
		check(rates["null "+w]["high"] <= 0.05, "null %s: high %.2f > 0.05", w, rates["null "+w]["high"])
		check(rates["null "+w]["growing"] <= 0.10, "null %s: growing %.2f > 0.10", w, rates["null "+w]["growing"])
		check(rates["null+spike "+w]["high"] <= 0.05, "null+spike %s: high %.2f", w, rates["null+spike "+w]["high"])
		check(rates["+30%/yr low volume "+w]["high"] == 0, "low volume must never be high")
	}
	// Power target holds for the default 36-month window only; 24 months are
	// too short (≈36%), which is why the report warns about short windows.
	check(rates["+15%/yr w36"]["high"] >= 0.70, "+15%% w36: high %.2f < 0.70", rates["+15%/yr w36"]["high"])
	check(rates["+30%/yr w24"]["growing"] >= 0.50, "+30%% w24: growing %.2f < 0.50", rates["+30%/yr w24"]["growing"])
	_ = fmt.Sprint
}
