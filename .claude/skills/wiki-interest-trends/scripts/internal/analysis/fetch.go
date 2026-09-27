package analysis

import (
	"time"

	"wikitrend/internal/spec"
	"wikitrend/internal/wiki"
)

// LangData is everything downloaded for one language edition.
type LangData struct {
	Months  []string             `json:"months"`   // YYYY-MM, ascending
	Agg     []float64            `json:"agg"`      // monthly views of the whole edition (agent=user)
	Days    []string             `json:"days"`     // YYYY-MM-DD, ascending
	Total   []float64            `json:"total"`    // daily views of the topic (all articles + redirects)
	PerItem map[string][]float64 `json:"per_item"` // QID -> daily views (article + its redirects)
}

// FetchMonths is how much history we download: the analysis window plus room
// for the sensitivity checks (window shifted by 3 months, 36-month window).
// It is at least 39 months for every window ≤ 36, so the URLs do not change
// when a follow-up only changes the window and everything comes from cache.
func FetchMonths(s *spec.Spec) int { return max(s.WindowMonths+3, 39) }

// Period returns the first and last month to download.
func Period(s *spec.Spec) (from, to time.Time) {
	to, _ = spec.ParseMonth(s.End)
	from = to.AddDate(0, -(FetchMonths(s) - 1), 0)
	if from.Before(spec.FirstMonth) {
		from = spec.FirstMonth
	}
	return from, to
}

// Fetch downloads (or reads from cache) all series needed for s.
func Fetch(c *wiki.Client, s *spec.Spec) (map[string]*LangData, error) {
	from, toMonth := Period(s)
	to := toMonth.AddDate(0, 1, -1) // last day of the end month
	var days []string
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format("2006-01-02"))
	}
	var months []string
	for m := from; !m.After(toMonth); m = m.AddDate(0, 1, 0) {
		months = append(months, m.Format("2006-01"))
	}

	res := map[string]*LangData{}
	for _, lang := range s.Langs {
		agg, err := c.ProjectMonthly(lang, s.Access, from, toMonth)
		if err != nil {
			return nil, err
		}
		ld := &LangData{Months: months, Days: days, Agg: make([]float64, len(months)),
			Total: make([]float64, len(days)), PerItem: map[string][]float64{}}
		for i, m := range months {
			ld.Agg[i] = float64(agg[m])
		}
		for _, a := range s.Articles[lang] {
			item := make([]float64, len(days))
			for _, title := range append([]string{a.Title}, a.Redirects...) {
				d, err := c.ArticleDaily(lang, s.Access, title, from, to)
				if err != nil {
					return nil, err
				}
				for i, day := range days {
					item[i] += float64(d[day]) // missing day = 0 views
				}
			}
			ld.PerItem[a.QID] = item
			for i := range days {
				ld.Total[i] += item[i]
			}
		}
		res[lang] = ld
	}
	return res, nil
}
