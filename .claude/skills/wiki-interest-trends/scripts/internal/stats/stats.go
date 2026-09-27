// Package stats implements the robust statistics the trust level is built on.
// Every function is checked against scipy / pymannkendall reference values
// (see testdata/reference.json and dev/gen_fixtures.py).
package stats

import (
	"math"
	"sort"
)

// Median returns the median of x (NaN for empty input). x is not modified.
func Median(x []float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// MAD is the unscaled median absolute deviation from the median.
func MAD(x []float64) float64 {
	m := Median(x)
	d := make([]float64, len(x))
	for i, v := range x {
		d[i] = math.Abs(v - m)
	}
	return Median(d)
}

// Quantile uses linear interpolation between order statistics (numpy default, "type 7").
func Quantile(x []float64, q float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// Hampel flags points further than t·σ from the median of a ±k window, where
// σ = 1.4826·MAD (floored at 1 so that all-zero windows still catch bursts),
// and replaces them by that median. Used on daily views to strip one-off spikes.
func Hampel(x []float64, k int, t float64) (clean []float64, flags []bool) {
	clean = append([]float64(nil), x...)
	flags = make([]bool, len(x))
	for i := range x {
		lo, hi := max(0, i-k), min(len(x), i+k+1)
		w := x[lo:hi]
		med := Median(w)
		sigma := math.Max(1.4826*MAD(w), 1)
		if math.Abs(x[i]-med) > t*sigma {
			clean[i] = med
			flags[i] = true
		}
	}
	return clean, flags
}

// TheilSen returns the median of pairwise slopes and the intercept
// median(y − slope·x) (scipy's method="joint"). O(n²), fine for n ≤ a few hundred.
func TheilSen(x, y []float64) (slope, intercept float64) {
	n := len(x)
	slopes := make([]float64, 0, n*(n-1)/2)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if x[j] != x[i] {
				slopes = append(slopes, (y[j]-y[i])/(x[j]-x[i]))
			}
		}
	}
	if len(slopes) == 0 {
		return math.NaN(), math.NaN()
	}
	slope = Median(slopes)
	res := make([]float64, n)
	for i := range x {
		res[i] = y[i] - slope*x[i]
	}
	return slope, Median(res)
}

// MK is the result of a (seasonal) Mann–Kendall test.
type MK struct {
	S    float64 `json:"s"`
	VarS float64 `json:"var_s"`
	Z    float64 `json:"z"`
	P    float64 `json:"p"` // two-sided
}

func mkSVar(y []float64) (s, v float64) {
	n := len(y)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			switch {
			case y[j] > y[i]:
				s++
			case y[j] < y[i]:
				s--
			}
		}
	}
	// Tie correction: Σ t(t−1)(2t+5) over groups of equal values.
	counts := map[float64]int{}
	for _, v := range y {
		counts[v]++
	}
	fn := float64(n)
	v = fn * (fn - 1) * (2*fn + 5)
	for _, c := range counts {
		t := float64(c)
		v -= t * (t - 1) * (2*t + 5)
	}
	return s, v / 18
}

func finish(s, v float64) MK {
	var z float64
	switch {
	case s > 0:
		z = (s - 1) / math.Sqrt(v)
	case s < 0:
		z = (s + 1) / math.Sqrt(v)
	}
	if v == 0 {
		z = 0
	}
	return MK{S: s, VarS: v, Z: z, P: 2 * (1 - NormCDF(math.Abs(z)))}
}

// MannKendall is the original MK trend test with tie correction.
func MannKendall(y []float64) MK {
	s, v := mkSVar(y)
	return finish(s, v)
}

// SeasonalMannKendall sums MK statistics over each season (e.g. period 12 for
// monthly data), so a yearly cycle is not mistaken for a trend.
func SeasonalMannKendall(y []float64, period int) MK {
	var S, V float64
	for s := 0; s < period; s++ {
		var sub []float64
		for i := s; i < len(y); i += period {
			sub = append(sub, y[i])
		}
		a, b := mkSVar(sub)
		S += a
		V += b
	}
	return finish(S, V)
}

// NormCDF is the standard normal CDF.
func NormCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// NormPPF is the inverse of NormCDF (bisection; accurate to ~1e-12).
func NormPPF(p float64) float64 {
	lo, hi := -40.0, 40.0
	for range 200 {
		mid := (lo + hi) / 2
		if NormCDF(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// SenCI is the confidence interval for the Theil–Sen slope from Sen (1968),
// eq. 2.6 — the same rule as scipy.stats.theilslopes. conf is e.g. 0.90.
// varFactor ≥ 1 widens the interval for autocorrelated data (see ACFactor);
// pass 1 for independent observations.
func SenCI(x, y []float64, conf, varFactor float64) (lo, hi float64) {
	n := len(y)
	var slopes []float64
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if x[j] != x[i] {
				slopes = append(slopes, (y[j]-y[i])/(x[j]-x[i]))
			}
		}
	}
	if len(slopes) == 0 {
		return math.NaN(), math.NaN()
	}
	sort.Float64s(slopes)
	fn := float64(n)
	sigsq := fn*(fn-1)*(2*fn+5) - tieTerm(x) - tieTerm(y)
	sigsq = sigsq / 18 * varFactor
	z := math.Abs(NormPPF((1 - conf) / 2))
	nt := float64(len(slopes))
	sigma := math.Sqrt(sigsq)
	// math.RoundToEven matches numpy's np.round.
	ru := min(int(math.RoundToEven((nt+z*sigma)/2)), len(slopes)-1)
	rl := max(int(math.RoundToEven((nt-z*sigma)/2))-1, 0)
	return slopes[rl], slopes[ru]
}

func tieTerm(v []float64) float64 {
	counts := map[float64]int{}
	for _, a := range v {
		counts[a]++
	}
	var s float64
	for _, c := range counts {
		if c > 1 {
			t := float64(c)
			s += t * (t - 1) * (2*t + 5)
		}
	}
	return s
}

// Lag1 is the lag-1 autocorrelation of x.
func Lag1(x []float64) float64 {
	n := len(x)
	if n < 3 {
		return 0
	}
	var m float64
	for _, v := range x {
		m += v
	}
	m /= float64(n)
	var num, den float64
	for i := range n {
		d := x[i] - m
		den += d * d
		if i > 0 {
			num += d * (x[i-1] - m)
		}
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// ACFactor is the variance inflation (1+r)/(1−r) for positive lag-1
// autocorrelation r of the detrended series (effective sample size idea of
// Yue & Wang, 2004). Neighbouring months of pageviews are correlated; without
// this the MK test and Sen interval are overconfident. Capped at 5.
func ACFactor(residuals []float64) float64 {
	r := Lag1(residuals)
	if r <= 0 {
		return 1
	}
	r = min(r, 0.66)
	return (1 + r) / (1 - r)
}

// WithVarFactor recomputes Z and p of an MK result with inflated variance.
func (m MK) WithVarFactor(f float64) MK {
	return finish(m.S, m.VarS*f)
}
