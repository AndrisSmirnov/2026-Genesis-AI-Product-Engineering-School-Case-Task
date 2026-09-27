package report

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"wikitrend/internal/analysis"
	"wikitrend/internal/out"
)

var values = map[string]string{"pl.trend": "+12.4%", "pl.trust": "medium", "pl.verdict": "growing", "topic": "Post przerywany", "period.months": "36"}

func code(err error) string {
	var e *out.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return fmt.Sprint(err)
}

func TestRenderSubstitutes(t *testing.T) {
	n, err := Render("# Interest {{pl.verdict}}\n\nShare trend {{ pl.trend }} per year, trust {{pl.trust}}.", values, "")
	if err != nil {
		t.Fatal(err)
	}
	if n.Headline != "Interest growing" || n.Body != "Share trend +12.4% per year, trust medium." {
		t.Fatalf("%+v", n)
	}
}

func TestRenderRejectsRawNumbers(t *testing.T) {
	for _, text := range []string{
		"Headline\nInterest grew by 12% last year.",
		"Headline\nOver the last 2 years.",
		"Headline {{pl.trend}}\nIn 2025 interest rose.",
		"Headline\nЗросло на ٣ відсотки.", // non-ASCII digits too
	} {
		if _, err := Render(text, values, ""); code(err) != "RAW_NUMBER" {
			t.Errorf("%q: want RAW_NUMBER, got %v", text, err)
		}
	}
}

func TestRenderRejectsUnknownPlaceholder(t *testing.T) {
	if _, err := Render("H\n{{cs.trend}}", values, ""); code(err) != "UNKNOWN_PLACEHOLDER" {
		t.Fatalf("got %v", err)
	}
}

func TestRenderNeedsBody(t *testing.T) {
	if _, err := Render("only a headline", values, ""); code(err) != "BAD_NARRATIVE" {
		t.Fatalf("got %v", err)
	}
}

func TestLocalize(t *testing.T) {
	v := Localize(values, "uk")
	if v["pl.trust"] != "середня" || v["pl.verdict"] != "зростає" || v["pl.trend"] != "+12.4%" {
		t.Fatalf("%v", v)
	}
}

func fakeResult() *analysis.Result {
	r := &analysis.Result{RunID: "r_test", Topic: "Post przerywany", Access: "all-access",
		Window:  analysis.Window{Start: "2023-09", End: "2026-08", Months: 36},
		Missing: map[string][]string{"sk": {"Q1"}}}
	titles := map[string]string{"pl": "Post przerywany (Łódź ąęśźż)", "cs": "Přerušovaný půst (ěščřžýáíéůú)", "uk": "Інтервальне голодування"}
	for i, lang := range []string{"pl", "cs", "uk"} {
		l := &analysis.LangResult{Lang: lang, Titles: []string{titles[lang]}, MonthlyViewsMed: float64(5000 * (i + 1)),
			ShareTrend: analysis.Trend{Est: 15 - 10*float64(i), CI90: [2]float64{5 - 10*float64(i), 25 - 10*float64(i)}},
			Verdict:    []string{"growing", "no_clear_trend", "declining"}[i], Trust: []string{"high", "medium", "low"}[i],
			Reasons:  []analysis.Reason{{Code: "spikes", Params: map[string]string{"month": "2025-01", "clean": "+1.0", "raw": "+9.0"}}},
			Warnings: []analysis.Reason{{Code: "crosses_bot_break"}}}
		yoy := 12.0
		l.ShareYoYPct = &yoy
		for m := 0; m < 36; m++ {
			d := time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, m, 0)
			l.Months = append(l.Months, d.Format("2006-01"))
			v := 10 * math.Pow(1+l.ShareTrend.Est/100, float64(m)/12)
			l.ShareClean = append(l.ShareClean, v)
			l.ShareRaw = append(l.ShareRaw, v*(1+0.8*b2f(m == 20)))
		}
		r.Langs = append(r.Langs, l)
		r.Ranking = append(r.Ranking, lang)
	}
	return r
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

var pageRe = regexp.MustCompile(`/Type\s*/Page[^s]`)

func TestBuildOnePageWithDiacritics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.pdf")
	n := Narrative{Headline: "Zainteresowanie rośnie: Příliš žluťoučký kůň úpěl ďábelské ódy — Інтерес зростає",
		Body: strings.Repeat("Частка переглядів зростає стабільно; łódź, ěščřž. ", 15)}
	if err := Build(fakeResult(), n, Options{Lang: "uk", Path: path, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(pageRe.FindAll(b, -1)); got != 1 {
		t.Fatalf("PDF has %d pages, want 1", got)
	}
	if !bytes.Contains(b, []byte("/FontFile2")) { // embedded TrueType program
		t.Fatal("font not embedded")
	}
}

func TestBuildFailsInsteadOfSecondPage(t *testing.T) {
	n := Narrative{Headline: "H", Body: strings.Repeat("Дуже довгий текст висновку. ", 200)}
	err := Build(fakeResult(), n, Options{Lang: "uk", Path: filepath.Join(t.TempDir(), "r.pdf"), Now: time.Now()})
	if code(err) != "NARRATIVE_TOO_LONG" {
		t.Fatalf("got %v", err)
	}
}

func TestChartSVG(t *testing.T) {
	svg := ChartSVG(fakeResult(), "title", "band")
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, ">pl<") || !strings.Contains(svg, "band") {
		t.Fatal("chart svg incomplete")
	}
}

func TestFontSafe(t *testing.T) {
	got := fontSafe("✅ Інтерес ≠ готовність платити\nДругий  рядок ⚠️", lbl("uk"))
	if got != "Інтерес не дорівнює готовність платити\nДругий рядок" {
		t.Fatalf("%q", got)
	}
}

func TestIndexIgnoresMonthsBeforeArticle(t *testing.T) {
	v := []float64{0, 0, 0, 10, 10, 10, 20}
	idx := indexSeries(v)
	if !math.IsNaN(idx[0]) || idx[3] != 100*10/indexBase(v) || indexBase(v) != 12.5 {
		t.Fatalf("idx %v base %v", idx, indexBase(v))
	}
}

func TestDetectLangAndDoublePercent(t *testing.T) {
	if DetectLang("Інтерес {{pl.trend}} зростає") != "uk" || DetectLang("Interest {{pl.trend}} grows") != "en" {
		t.Fatal("language detection")
	}
	n, err := Render("H\nTrend {{pl.trend}}% per year", values, "")
	if err != nil || n.Body != "Trend +12.4% per year" {
		t.Fatalf("%q %v", n.Body, err)
	}
}
