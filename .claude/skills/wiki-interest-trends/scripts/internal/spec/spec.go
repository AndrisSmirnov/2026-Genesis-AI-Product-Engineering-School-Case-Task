// Package spec is the reproducible description of one analysis. A follow-up
// question ("and for 3 years?") becomes a child spec with one field changed.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"wikitrend/internal/out"
)

// Item is one Wikidata entity in the topic basket.
type Item struct {
	QID         string `json:"qid"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Article is the page for one entity in one language, plus its redirects.
type Article struct {
	QID       string   `json:"qid"`
	Title     string   `json:"title"`
	Redirects []string `json:"redirects,omitempty"`
}

// Criteria are the user's thresholds for "promising", applied by code.
type Criteria struct {
	MinGrowthPct    float64 `json:"min_growth_pct"`    // trend per year, share-based
	MinMonthlyViews float64 `json:"min_monthly_views"` // median monthly views of the topic
}

// Spec fully determines an analysis run.
type Spec struct {
	Version      int                  `json:"version"`
	Topic        string               `json:"topic"`
	Items        []Item               `json:"items"`
	Langs        []string             `json:"langs"`
	Articles     map[string][]Article `json:"articles"`
	Missing      map[string][]string  `json:"missing,omitempty"` // lang -> QIDs without an article
	End          string               `json:"end"`               // last included month, YYYY-MM
	WindowMonths int                  `json:"window_months"`
	Access       string               `json:"access"`
	Criteria     Criteria             `json:"criteria"`
	ResolvedAt   string               `json:"resolved_at"`
	ParentRunID  string               `json:"parent_run_id,omitempty"`
}

// Defaults.
const (
	DefaultWindow = 36 // 24 months miss moderate growth too often (calibrate_test.go)
	MinWindow     = 24
	MaxWindow     = 120
)

// DefaultCriteria are hypotheses to be calibrated (see references/METHODOLOGY.md).
var DefaultCriteria = Criteria{MinGrowthPct: 10, MinMonthlyViews: 1000}

// FirstMonth is the first month with per-article pageview data.
var FirstMonth = time.Date(2015, 7, 1, 0, 0, 0, 0, time.UTC)

// LastCompleteMonth returns the last month whose data is surely loaded
// (Wikimedia can lag 24h+, so the first 2 days of a month still count as the old one).
func LastCompleteMonth(now time.Time) time.Time {
	now = now.UTC().AddDate(0, 0, -2)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
}

// ParseMonth parses YYYY-MM.
func ParseMonth(s string) (time.Time, error) {
	return time.Parse("2006-01", s)
}

// RunID is a content hash: the same spec always maps to the same run.
func (s *Spec) RunID() string {
	c := *s
	c.ResolvedAt, c.ParentRunID = "", ""
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return "r_" + hex.EncodeToString(h[:])[:8]
}

// Validate checks the spec and fills defaults.
func (s *Spec) Validate(now time.Time) error {
	if s.Version == 0 {
		s.Version = 1
	}
	if s.WindowMonths == 0 {
		s.WindowMonths = DefaultWindow
	}
	if s.Access == "" {
		s.Access = "all-access"
	}
	if s.Criteria == (Criteria{}) {
		s.Criteria = DefaultCriteria
	}
	if s.End == "" {
		s.End = LastCompleteMonth(now).Format("2006-01")
	}
	fix := "Fix the value and rerun."
	end, err := ParseMonth(s.End)
	if err != nil {
		return out.Errf("BAD_ARGS", fix, "end must be YYYY-MM, got %q", s.End)
	}
	if last := LastCompleteMonth(now); end.After(last) {
		return out.Errf("BAD_ARGS", fix, "end %s is not complete yet; latest complete month is %s", s.End, last.Format("2006-01"))
	}
	if s.WindowMonths < MinWindow || s.WindowMonths > MaxWindow {
		return out.Errf("BAD_ARGS", fix, "window_months must be %d..%d, got %d", MinWindow, MaxWindow, s.WindowMonths)
	}
	if start := end.AddDate(0, -(s.WindowMonths - 1), 0); start.Before(FirstMonth) {
		return out.Errf("BAD_ARGS", fix, "window starts %s, but pageview data begins 2015-07", start.Format("2006-01"))
	}
	if !slices.Contains([]string{"all-access", "desktop", "mobile-web", "mobile-app"}, s.Access) {
		return out.Errf("BAD_ARGS", fix, "access must be all-access|desktop|mobile-web|mobile-app, got %q", s.Access)
	}
	if len(s.Langs) == 0 {
		return out.Errf("BAD_ARGS", fix, "no languages in spec")
	}
	return nil
}

// Set applies one "key=value" override, as used by `analyze --from <run> --set`.
func (s *Spec) Set(kv string) error {
	k, v, ok := strings.Cut(kv, "=")
	if !ok {
		return out.Errf("BAD_ARGS", "Use --set key=value, e.g. --set window_months=36.", "bad --set %q", kv)
	}
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	num := func() (float64, error) {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, out.Errf("BAD_ARGS", "Use a number.", "%s must be a number, got %q", k, v)
		}
		return f, nil
	}
	switch k {
	case "window_months", "window.months", "months":
		f, err := num()
		if err != nil {
			return err
		}
		s.WindowMonths = int(f)
	case "end":
		s.End = v
	case "access":
		s.Access = v
	case "criteria.min_growth_pct", "min_growth_pct":
		f, err := num()
		if err != nil {
			return err
		}
		s.Criteria.MinGrowthPct = f
	case "criteria.min_monthly_views", "min_monthly_views":
		f, err := num()
		if err != nil {
			return err
		}
		s.Criteria.MinMonthlyViews = f
	case "langs":
		langs := SplitList(v)
		var missing []string
		for _, l := range langs {
			if _, ok := s.Articles[l]; !ok && !slices.Contains(s.Langs, l) {
				missing = append(missing, l)
			}
		}
		if len(missing) > 0 {
			qids := make([]string, len(s.Items))
			for i, it := range s.Items {
				qids[i] = it.QID
			}
			return out.Errf("NEEDS_RESOLVE",
				fmt.Sprintf("Run: resolve --qid %s --langs %s", strings.Join(qids, ","), strings.Join(langs, ",")),
				"languages %v were not resolved in this spec", missing)
		}
		s.Langs = langs
	default:
		return out.Errf("BAD_ARGS", "Allowed keys: window_months, end, access, langs, min_growth_pct, min_monthly_views.", "unknown key %q", k)
	}
	return nil
}

// SplitList splits "a, b,c" into ["a","b","c"], lower-cased, without empties.
func SplitList(s string) []string {
	var res []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" && !slices.Contains(res, p) {
			res = append(res, p)
		}
	}
	return res
}
