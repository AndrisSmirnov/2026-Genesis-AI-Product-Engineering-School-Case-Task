// Package report builds the chart and the one-page PDF.
package report

import (
	_ "embed"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/signintech/gopdf"

	"wikitrend/internal/analysis"
	"wikitrend/internal/httpx"
	"wikitrend/internal/out"
)

//go:embed fonts/NotoSans-Regular.ttf
var fontRegular []byte

//go:embed fonts/NotoSans-Bold.ttf
var fontBold []byte

const (
	pageW, pageH = 595.28, 841.89 // A4
	margin       = 36.0
	contentW     = pageW - 2*margin
)

type pdfCanvas struct{ p *gopdf.GoPdf }

func (c *pdfCanvas) Line(x1, y1, x2, y2 float64, col rgb, w float64) {
	c.p.SetStrokeColor(col[0], col[1], col[2])
	c.p.SetLineWidth(w)
	c.p.Line(x1, y1, x2, y2)
}

func (c *pdfCanvas) Rect(x, y, w, h float64, fill rgb) {
	c.p.SetFillColor(fill[0], fill[1], fill[2])
	c.p.RectFromUpperLeftWithStyle(x, y, w, h, "F")
}

func (c *pdfCanvas) Text(x, y float64, s string, size float64, col rgb, anchor string) {
	_ = c.p.SetFont("reg", "", size)
	c.p.SetTextColor(col[0], col[1], col[2])
	w, _ := c.p.MeasureTextWidth(s)
	switch anchor {
	case "middle":
		x -= w / 2
	case "end":
		x -= w
	}
	c.p.SetXY(x, y-size*0.62)
	_ = c.p.Cell(nil, s)
}

// writer tracks the vertical cursor on the single page.
type writer struct {
	p *gopdf.GoPdf
	y float64
}

func (w *writer) font(bold bool, size float64, c rgb) {
	f := "reg"
	if bold {
		f = "bold"
	}
	_ = w.p.SetFont(f, "", size)
	w.p.SetTextColor(c[0], c[1], c[2])
}

// para writes wrapped text and advances the cursor.
func (w *writer) para(x, width float64, text string, size, lead float64) {
	for _, raw := range strings.Split(text, "\n") {
		if strings.TrimSpace(raw) == "" {
			w.y += lead * 0.4
			continue
		}
		lines, err := w.p.SplitTextWithWordWrap(raw, width)
		if err != nil {
			lines = []string{raw}
		}
		for _, l := range lines {
			w.p.SetXY(x, w.y)
			_ = w.p.Cell(nil, l)
			w.y += lead
		}
	}
	_ = size
}

func (w *writer) fit(s string, width float64) string {
	if tw, _ := w.p.MeasureTextWidth(s); tw <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if tw, _ := w.p.MeasureTextWidth(string(r) + "…"); tw <= width {
			return string(r) + "…"
		}
	}
	return s
}

var trustColor = map[string]rgb{"high": {22, 128, 61}, "medium": {180, 110, 0}, "low": {200, 30, 30}}

// Options configure one report.
type Options struct {
	Lang string // label language: uk | en
	Path string // output PDF path
	Now  time.Time
	Next string // next_step for errors
}

// Build renders the one-page PDF. It first shrinks the chart to make room and
// fails (instead of adding a second page) only if the text still does not fit.
func Build(r *analysis.Result, n Narrative, o Options) error {
	var overflow float64
	for _, h := range []float64{215, 185, 160, 140} {
		var err error
		overflow, err = buildOnce(r, n, o, h)
		if err != nil || overflow <= 0 {
			return err
		}
	}
	return out.Errf("NARRATIVE_TOO_LONG", o.Next,
		"the report does not fit on one page; shorten the narrative body by about %d characters (keep the headline)",
		int(math.Ceil(overflow/12.5*95))+60)
}

// buildOnce lays the page out with the given chart height. It returns how many
// points the content overflows (and writes nothing), or 0 after writing the PDF.
func buildOnce(r *analysis.Result, n Narrative, o Options, chartH float64) (float64, error) {
	L := lbl(o.Lang)
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: pageH}})
	p.SetInfo(gopdf.PdfInfo{Title: n.Headline, Creator: "wikitrend " + httpx.Version})
	if err := p.AddTTFFontData("reg", fontRegular); err != nil {
		return 0, err
	}
	if err := p.AddTTFFontData("bold", fontBold); err != nil {
		return 0, err
	}
	p.AddPage()
	w := &writer{p: p, y: margin}

	// Headline + subtitle.
	w.font(false, 8, muted)
	w.para(margin, contentW, strings.ToUpper(L.Subtitle), 8, 11)
	w.font(true, 15, ink)
	w.para(margin, contentW, n.Headline, 15, 19)
	w.y += 2
	var qids []string
	for _, it := range r.Items {
		qids = append(qids, it.QID)
	}
	w.font(false, 8.5, muted)
	w.para(margin, contentW, fmt.Sprintf("%s: %s (Wikidata %s) · %s: %s · %s: %s – %s (%d %s) · %s, %s",
		L.Topic, r.Topic, strings.Join(qids, ", "), L.Langs, strings.Join(langsOf(r), ", "),
		L.Period, r.Window.Start, r.Window.End, r.Window.Months, L.Months, L.Source, r.Access), 8.5, 11)
	w.y += 6

	// Table.
	cols := []struct {
		title string
		w     float64
	}{{L.ColLang, 34}, {L.ColArticle, 118}, {L.ColViews, 72}, {L.ColYoY, 56}, {L.ColTrend, 112}, {L.ColVerdict, 67}, {L.ColTrust, 0}}
	used := 0.0
	for _, c := range cols[:len(cols)-1] {
		used += c.w
	}
	cols[len(cols)-1].w = contentW - used
	const rowH = 15.0
	cv := &pdfCanvas{p}
	cv.Rect(margin, w.y, contentW, rowH, rgb{243, 244, 246})
	x := margin
	w.font(true, 7.5, ink)
	for _, c := range cols {
		p.SetXY(x+3, w.y+4)
		_ = p.Cell(nil, w.fit(c.title, c.w-5))
		x += c.w
	}
	w.y += rowH
	byLang := map[string]*analysis.LangResult{}
	for _, l := range r.Langs {
		byLang[l.Lang] = l
	}
	for i, lang := range r.Ranking {
		l := byLang[lang]
		if i%2 == 1 {
			cv.Rect(margin, w.y, contentW, rowH, rgb{249, 250, 251})
		}
		yoy := "—"
		if l.ShareYoYPct != nil {
			yoy = fmt.Sprintf("%+.1f%%", *l.ShareYoYPct)
		}
		cells := []string{l.Lang, strings.Join(l.Titles, ", "), fmtCount(l.MonthlyViewsMed), yoy,
			fmt.Sprintf("%+.1f%% [%+.0f; %+.0f]", l.ShareTrend.Est, l.ShareTrend.CI90[0], l.ShareTrend.CI90[1]),
			L.Verdict[l.Verdict], L.Trust[l.Trust]}
		x = margin
		for j, c := range cols {
			if j == len(cols)-1 {
				w.font(true, 8, trustColor[l.Trust])
			} else {
				w.font(j == 0, 8, ink)
			}
			p.SetXY(x+3, w.y+4)
			_ = p.Cell(nil, w.fit(cells[j], c.w-5))
			x += c.w
		}
		w.y += rowH
	}
	if len(r.Missing) > 0 {
		w.font(false, 8, muted)
		p.SetXY(margin+3, w.y+4)
		_ = p.Cell(nil, w.fit(fmt.Sprintf("%s: %s", L.Missing, missingText(r)), contentW-6))
		w.y += rowH
	}
	cv.Line(margin, w.y, margin+contentW, w.y, grid, 0.6)
	w.y += 10

	// Chart.
	w.font(true, 8.5, ink)
	w.para(margin, contentW, L.ChartTitle, 8.5, 11)
	drawChart(cv, r, margin, w.y, contentW, chartH, L.BandLabel)
	w.y += chartH + 8

	// Narrative.
	w.font(true, 11, ink)
	w.para(margin, contentW, L.Conclusion, 11, 15)
	w.font(false, 9.5, ink)
	w.para(margin, contentW, n.Body, 9.5, 12.5)
	w.y += 6

	// Trust reasons (from code, not from the model).
	w.font(true, 10, ink)
	w.para(margin, contentW, L.WhyTrust, 10, 14)
	for _, lang := range r.Ranking {
		l := byLang[lang]
		var rs []string
		maxR := 3
		if len(r.Langs) > 3 {
			maxR = 2
		}
		for i, rr := range l.Reasons {
			if i == maxR {
				break
			}
			rs = append(rs, reasonText(L, rr))
		}
		w.font(false, 8.5, ink)
		w.para(margin, contentW, fmt.Sprintf("%s — %s: %s.", l.Lang, L.Trust[l.Trust], strings.Join(rs, "; ")), 8.5, 11)
	}
	w.y += 5

	// Next checks.
	w.font(true, 10, ink)
	w.para(margin, contentW, L.NextChecks, 10, 14)
	w.font(false, 8.5, ink)
	for _, s := range nextChecks(r, L) {
		w.para(margin, contentW, "• "+s, 8.5, 11)
	}

	// Footer: caveats + assumptions, anchored to the bottom.
	var caveats []string
	seen := map[string]bool{}
	for _, lang := range r.Ranking {
		for _, wr := range byLang[lang].Warnings {
			t := reasonText(L, wr)
			if !seen[t] {
				seen[t] = true
				caveats = append(caveats, t)
			}
		}
	}
	footer := append([]string{}, L.AssumptionLines...)
	if len(caveats) > 0 {
		footer = append([]string{L.Caveats + ": " + strings.Join(caveats, "; ") + "."}, footer...)
	}
	footer = append(footer, fmt.Sprintf("%s: wikitrend %s · %s · run %s · data CC0 Wikimedia Foundation",
		L.Generated, httpx.Version, o.Now.Format("2006-01-02"), r.RunID))

	fw := &writer{p: p}
	fw.font(false, 7, muted)
	footerH := 13.0 // title line
	for _, f := range footer {
		lines, _ := p.SplitTextWithWordWrap(f, contentW)
		footerH += float64(len(lines)) * 9
	}
	top := pageH - margin - footerH
	if w.y > top-6 {
		return w.y - (top - 6), nil
	}
	cv.Line(margin, top-4, margin+contentW, top-4, grid, 0.6)
	fw.y = top
	fw.font(true, 7.5, muted)
	fw.para(margin, contentW, L.Assumptions, 7.5, 11)
	fw.font(false, 7, muted)
	for _, f := range footer {
		fw.para(margin, contentW, f, 7, 9)
	}
	return 0, p.WritePdf(o.Path)
}

// missingText lists languages without an article, e.g. "uk, pl (Q130192)".
func missingText(r *analysis.Result) string {
	byQ := map[string][]string{}
	for _, lang := range sortedKeys(r.Missing) {
		for _, q := range r.Missing[lang] {
			byQ[q] = append(byQ[q], lang)
		}
	}
	var parts []string
	for _, q := range sortedKeys(byQ) {
		label := q
		for _, it := range r.Items {
			if it.QID == q && it.Label != "" {
				label = it.Label
			}
		}
		parts = append(parts, fmt.Sprintf("%s — %s", label, strings.Join(byQ[q], ", ")))
	}
	return strings.Join(parts, "; ")
}

func nextChecks(r *analysis.Result, L Labels) []string {
	var res []string
	add := func(k string, kv ...string) {
		t := L.Next[k]
		for i := 0; i+1 < len(kv); i += 2 {
			t = strings.ReplaceAll(t, "{"+kv[i]+"}", kv[i+1])
		}
		for _, x := range res {
			if x == t {
				return
			}
		}
		res = append(res, t)
	}
	for _, l := range r.Langs {
		for _, rr := range append(append([]analysis.Reason{}, l.Reasons...), l.Warnings...) {
			switch rr.Code {
			case "low_volume", "short_window", "bot_break":
				add(rr.Code)
			case "spikes":
				add("spikes", "month", rr.Params["month"])
			}
		}
	}
	if len(r.Missing) > 0 {
		add("missing", "langs", strings.Join(sortedKeys(r.Missing), ", "))
	}
	if len(res) > 2 {
		res = res[:2]
	}
	add("validate")
	return res
}

func langsOf(r *analysis.Result) []string {
	var s []string
	for _, l := range r.Langs {
		s = append(s, l.Lang)
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}

func fmtCount(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
