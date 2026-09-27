package stats

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"testing"
)

type refCase struct {
	Name     string    `json:"name"`
	Y        []float64 `json:"y"`
	Median   float64   `json:"median"`
	MAD      float64   `json:"mad"`
	Q05      float64   `json:"q05"`
	Q95      float64   `json:"q95"`
	TheilSen struct {
		Slope, Intercept, Low90, High90 float64
	} `json:"theil_sen"`
	MK  *mkRef `json:"mk"`
	SMK *mkRef `json:"smk"`
}

type mkRef struct {
	S    float64 `json:"s"`
	VarS float64 `json:"var_s"`
	Z    float64 `json:"z"`
	P    float64 `json:"p"`
}

// Reference values come from scipy 1.13 / pymannkendall 1.4 (dev/gen_fixtures.py).
func loadRef(t *testing.T) []refCase {
	t.Helper()
	b, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Cases []refCase }
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r.Cases
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %.10g, want %.10g", what, got, want)
	}
}

func xs(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = float64(i)
	}
	return x
}

func TestAgainstReference(t *testing.T) {
	for _, c := range loadRef(t) {
		t.Run(c.Name, func(t *testing.T) {
			near(t, "median", Median(c.Y), c.Median, 1e-9)
			near(t, "mad", MAD(c.Y), c.MAD, 1e-9)
			near(t, "q05", Quantile(c.Y, 0.05), c.Q05, 1e-9)
			near(t, "q95", Quantile(c.Y, 0.95), c.Q95, 1e-9)

			slope, icpt := TheilSen(xs(len(c.Y)), c.Y)
			near(t, "theil-sen slope", slope, c.TheilSen.Slope, 1e-9)
			near(t, "theil-sen intercept", icpt, c.TheilSen.Intercept, 1e-9)
			lo, hi := SenCI(xs(len(c.Y)), c.Y, 0.90, 1)
			near(t, "sen ci low", lo, c.TheilSen.Low90, 1e-9)
			near(t, "sen ci high", hi, c.TheilSen.High90, 1e-9)

			m := MannKendall(c.Y)
			near(t, "mk.s", m.S, c.MK.S, 1e-9)
			near(t, "mk.var", m.VarS, c.MK.VarS, 1e-9)
			near(t, "mk.z", m.Z, c.MK.Z, 1e-9)
			near(t, "mk.p", m.P, c.MK.P, 1e-9)

			if c.SMK != nil {
				s := SeasonalMannKendall(c.Y, 12)
				near(t, "smk.s", s.S, c.SMK.S, 1e-9)
				near(t, "smk.var", s.VarS, c.SMK.VarS, 1e-9)
				near(t, "smk.z", s.Z, c.SMK.Z, 1e-9)
				near(t, "smk.p", s.P, c.SMK.P, 1e-9)
			}
		})
	}
}

func TestHampelRemovesSingleSpike(t *testing.T) {
	y := []float64{10, 11, 9, 10, 12, 10, 500, 11, 10, 9, 10, 11}
	clean, flags := Hampel(y, 3, 3)
	for i := range y {
		if i == 6 {
			if !flags[i] || clean[i] > 20 {
				t.Fatalf("spike not removed: flag=%v clean=%v", flags[i], clean[i])
			}
			continue
		}
		if flags[i] || clean[i] != y[i] {
			t.Fatalf("day %d changed: flag=%v clean=%v", i, flags[i], clean[i])
		}
	}
}

func TestHampelKeepsSmoothTrend(t *testing.T) {
	y := make([]float64, 60)
	for i := range y {
		y[i] = 100 + float64(i)
	}
	_, flags := Hampel(y, 3, 3)
	for i, f := range flags {
		if f {
			t.Fatalf("smooth trend flagged at %d", i)
		}
	}
}

func TestHampelAllZerosWithOneVisitBurst(t *testing.T) {
	// MAD = 0 → scale floor of 1 view keeps the filter meaningful.
	y := []float64{0, 0, 0, 0, 40, 0, 0, 0}
	_, flags := Hampel(y, 3, 3)
	if !flags[4] {
		t.Fatal("burst on zero baseline not flagged")
	}
}

func TestTheilSenIgnoresOutlier(t *testing.T) {
	x := xs(20)
	y := make([]float64, 20)
	for i := range y {
		y[i] = 2*x[i] + 1
	}
	y[10] = 1000
	slope, icpt := TheilSen(x, y)
	near(t, "slope", slope, 2, 1e-12)
	near(t, "intercept", icpt, 1, 1e-12)
}

func TestMannKendallNoTrendOnConstant(t *testing.T) {
	m := MannKendall([]float64{5, 5, 5, 5, 5, 5})
	if m.S != 0 || m.Z != 0 || m.P != 1 {
		t.Fatalf("constant series: %+v", m)
	}
}

// A 90% interval must contain the true slope in roughly 90% of simulated
// series. One series proves nothing, so we check coverage over many.
func coverage(t *testing.T, phi float64, correct bool) float64 {
	const sims, n, trueSlope = 400, 36, 0.02
	rng := rand.New(rand.NewSource(1))
	x := xs(n)
	covered := 0
	for range sims {
		y := make([]float64, n)
		e := 0.0
		for i := range y {
			e = phi*e + 0.05*rng.NormFloat64() // AR(1) noise
			y[i] = trueSlope*x[i] + e
		}
		f := 1.0
		if correct {
			s, c := TheilSen(x, y)
			res := make([]float64, n)
			for i := range y {
				res[i] = y[i] - (c + s*x[i])
			}
			f = ACFactor(res)
		}
		lo, hi := SenCI(x, y, 0.90, f)
		if lo <= trueSlope && trueSlope <= hi {
			covered++
		}
	}
	return float64(covered) / sims
}

func TestSenCICoverageIndependent(t *testing.T) {
	c := coverage(t, 0, false)
	t.Logf("iid noise: coverage %.2f", c)
	if c < 0.85 || c > 0.96 {
		t.Fatalf("coverage %.2f outside [0.85, 0.96]", c)
	}
}

func TestSenCICoverageAutocorrelated(t *testing.T) {
	raw, fixed := coverage(t, 0.5, false), coverage(t, 0.5, true)
	t.Logf("AR(1) φ=0.5: coverage raw %.2f, with ACFactor %.2f", raw, fixed)
	if fixed < 0.82 {
		t.Fatalf("corrected coverage %.2f < 0.82", fixed)
	}
	if fixed <= raw {
		t.Fatalf("correction must improve coverage: raw %.2f, fixed %.2f", raw, fixed)
	}
}
