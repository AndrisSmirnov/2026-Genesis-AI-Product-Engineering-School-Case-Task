package spec

import (
	"errors"
	"testing"
	"time"

	"wikitrend/internal/out"
)

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func base() *Spec {
	return &Spec{
		Topic: "astronomy", Items: []Item{{QID: "Q333"}}, Langs: []string{"uk"},
		Articles: map[string][]Article{"uk": {{QID: "Q333", Title: "Астрономія"}}},
	}
}

func TestLastCompleteMonth(t *testing.T) {
	cases := map[string]string{
		"2026-09-27": "2026-08",
		"2026-09-03": "2026-08",
		"2026-09-02": "2026-07", // data for August may still be loading
		"2026-01-01": "2025-11",
	}
	for day, want := range cases {
		d, _ := time.Parse("2006-01-02", day)
		if got := LastCompleteMonth(d).Format("2006-01"); got != want {
			t.Errorf("%s: got %s want %s", day, got, want)
		}
	}
}

func TestValidateDefaults(t *testing.T) {
	s := base()
	if err := s.Validate(now); err != nil {
		t.Fatal(err)
	}
	if s.End != "2026-08" || s.WindowMonths != 36 || s.Access != "all-access" || s.Criteria != DefaultCriteria {
		t.Fatalf("defaults not applied: %+v", s)
	}
}

func TestValidateRejects(t *testing.T) {
	for name, mut := range map[string]func(*Spec){
		"future end":     func(s *Spec) { s.End = "2026-09" },
		"too early":      func(s *Spec) { s.End = "2017-01"; s.WindowMonths = 36 },
		"tiny window":    func(s *Spec) { s.WindowMonths = 6 },
		"bad access":     func(s *Spec) { s.Access = "mobile" },
		"no languages":   func(s *Spec) { s.Langs = nil },
		"bad end format": func(s *Spec) { s.End = "08/2026" },
	} {
		s := base()
		mut(s)
		var e *out.Error
		if err := s.Validate(now); !errors.As(err, &e) || e.Code != "BAD_ARGS" {
			t.Errorf("%s: want BAD_ARGS, got %v", name, err)
		}
	}
}

func TestRunIDStableAndIgnoresLineage(t *testing.T) {
	a, b := base(), base()
	b.ParentRunID, b.ResolvedAt = "r_parent", "2026-01-01"
	if a.RunID() != b.RunID() {
		t.Fatal("lineage fields must not change run id")
	}
	b.WindowMonths = 36
	if a.RunID() == b.RunID() {
		t.Fatal("different window must change run id")
	}
}

func TestSet(t *testing.T) {
	s := base()
	for _, kv := range []string{"window_months=36", "access=desktop", "min_growth_pct=5", "end=2026-06"} {
		if err := s.Set(kv); err != nil {
			t.Fatal(kv, err)
		}
	}
	if s.WindowMonths != 36 || s.Access != "desktop" || s.Criteria.MinGrowthPct != 5 || s.End != "2026-06" {
		t.Fatalf("not applied: %+v", s)
	}
	var e *out.Error
	if err := s.Set("langs=uk,pl"); !errors.As(err, &e) || e.Code != "NEEDS_RESOLVE" {
		t.Fatalf("adding an unresolved language must ask for resolve, got %v", err)
	}
	if err := s.Set("colour=red"); !errors.As(err, &e) || e.Code != "BAD_ARGS" {
		t.Fatalf("unknown key: %v", err)
	}
}
