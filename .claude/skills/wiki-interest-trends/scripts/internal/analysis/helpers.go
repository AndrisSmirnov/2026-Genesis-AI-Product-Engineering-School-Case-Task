package analysis

import (
	"fmt"
	"math"
	"strings"
)

func toStrings(v []string) []string { return append([]string(nil), v...) }

// daysInLastMonths counts days that belong to the last k months of days.
func daysInLastMonths(days []string, k int) int {
	if len(days) == 0 {
		return 0
	}
	seen, n := 0, 0
	cur := ""
	for i := len(days) - 1; i >= 0; i-- {
		if m := days[i][:7]; m != cur {
			cur = m
			seen++
			if seen > k {
				break
			}
		}
		n++
	}
	return n
}

func joinOr(parts []string, empty string) string {
	if len(parts) == 0 {
		return empty
	}
	return strings.Join(parts, "; ")
}

func replaceAll(s, old, new string) string { return strings.ReplaceAll(s, old, new) }

func firstNonZero(v []float64) int {
	for i, x := range v {
		if x > 0 {
			return i
		}
	}
	return len(v)
}

// fmtPct formats a percentage with sign, e.g. "+12.4%".
func fmtPct(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", v)
}

// fmtInt formats a count with thin spaces between thousands, e.g. "18 400".
func fmtInt(v float64) string {
	s := fmt.Sprintf("%.0f", math.Round(v))
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// placeholders are the only way the model may put numbers into a report.
func placeholders(r *Result) map[string]string {
	p := map[string]string{
		"topic":         r.Topic,
		"period.start":  r.Window.Start,
		"period.end":    r.Window.End,
		"period.months": fmt.Sprint(r.Window.Months),
		"langs.count":   fmt.Sprint(len(r.Langs)),
	}
	for i, l := range r.Ranking {
		p[fmt.Sprintf("rank.%d", i+1)] = l
	}
	for _, l := range r.Langs {
		k := l.Lang + "."
		p[k+"title"] = strings.Join(l.Titles, ", ")
		p[k+"monthly_views"] = fmtInt(l.MonthlyViewsMed)
		p[k+"trend"] = fmtPct(l.ShareTrend.Est)
		p[k+"trend_ci"] = fmtPct(l.ShareTrend.CI90[0]) + " … " + fmtPct(l.ShareTrend.CI90[1])
		p[k+"share_yoy"] = "n/a"
		if l.ShareYoYPct != nil {
			p[k+"share_yoy"] = fmtPct(*l.ShareYoYPct)
		}
		p[k+"views_yoy"] = "n/a"
		if l.ViewsYoYPct != nil {
			p[k+"views_yoy"] = fmtPct(*l.ViewsYoYPct)
		}
		p[k+"edition_trend"] = fmtPct(l.EditionTrendPct)
		p[k+"trust"] = l.Trust
		p[k+"verdict"] = l.Verdict
		p[k+"spike_month"] = "—"
		if len(l.SpikeMonths) > 0 {
			p[k+"spike_month"] = l.SpikeMonths[len(l.SpikeMonths)-1]
		}
	}
	return p
}
