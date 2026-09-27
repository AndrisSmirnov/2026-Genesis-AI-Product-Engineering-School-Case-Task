package report

import (
	"fmt"
	"html"
	"math"
	"strings"

	"wikitrend/internal/analysis"
)

type rgb [3]uint8

var palette = []rgb{{37, 99, 235}, {220, 38, 38}, {22, 163, 74}, {217, 119, 6}, {147, 51, 234}, {8, 145, 178}, {219, 39, 119}, {75, 85, 99}}

var (
	ink   = rgb{17, 24, 39}
	muted = rgb{107, 114, 128}
	grid  = rgb{229, 231, 235}
	band  = rgb{243, 244, 246}
)

// canvas is implemented by the SVG writer and the PDF page, so the chart is
// drawn once and looks the same in both.
type canvas interface {
	Line(x1, y1, x2, y2 float64, c rgb, w float64)
	Rect(x, y, w, h float64, fill rgb)
	// Text draws s with its vertical centre at y; anchor is "start", "middle" or "end".
	Text(x, y float64, s string, size float64, c rgb, anchor string)
}

// indexBase is the mean of the first 12 months in which the article already
// had views. Months before a new article appeared are zeros and would make
// the index explode (a new Polish article made the line hit the ceiling).
func indexBase(v []float64) float64 {
	var base float64
	n := 0
	for _, x := range v {
		if x > 0 {
			base += x
			n++
			if n == 12 {
				break
			}
		}
	}
	return base / float64(max(n, 1))
}

// indexSeries rescales a share series so that indexBase is 100.
// Languages of very different size become comparable on one axis.
func indexSeries(v []float64) []float64 {
	base := indexBase(v)
	res := make([]float64, len(v))
	started := false
	for i, x := range v {
		started = started || x > 0
		if base > 0 && started {
			res[i] = x / base * 100
		} else {
			res[i] = math.NaN() // before the article existed: no line
		}
	}
	return res
}

func niceStep(span float64, target int) float64 {
	raw := span / float64(target)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}

// drawChart draws the indexed share of every language into the box (x, y, w, h).
func drawChart(c canvas, r *analysis.Result, x, y, w, h float64, bandLabel string) {
	const padL, padB, padT, padR = 34.0, 16.0, 6.0, 44.0
	px, py, pw, ph := x+padL, y+padT, w-padL-padR, h-padT-padB
	if len(r.Langs) == 0 {
		return
	}
	months := r.Langs[0].Months
	n := len(months)
	type line struct {
		clean, raw []float64
		col        rgb
		lang       string
	}
	var lines []line
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, l := range r.Langs {
		// Raw is indexed with the clean base so the spike overshoot is visible.
		cl := indexSeries(l.ShareClean)
		base := indexBase(l.ShareClean)
		rw := make([]float64, len(l.ShareRaw))
		for j, v := range l.ShareRaw {
			rw[j] = math.NaN()
			if base > 0 && !math.IsNaN(cl[j]) {
				rw[j] = v / base * 100
			}
		}
		lines = append(lines, line{cl, rw, palette[i%len(palette)], l.Lang})
		for _, v := range append(append([]float64{}, cl...), rw...) {
			if !math.IsNaN(v) {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
	}
	if math.IsInf(lo, 0) {
		lo, hi = 0, 200
	}
	lo = math.Min(lo, 100)
	hi = math.Max(hi, 100)
	// Cap extreme spikes so the trend stays readable.
	if hi > 4*math.Max(lo, 1) && hi > 400 {
		hi = 400
	}
	step := niceStep(hi-lo, 5)
	lo = math.Floor(lo/step) * step
	hi = math.Ceil(hi/step) * step
	sx := func(i int) float64 { return px + pw*float64(i)/float64(max(n-1, 1)) }
	sy := func(v float64) float64 {
		v = math.Max(lo, math.Min(hi, v))
		return py + ph*(1-(v-lo)/(hi-lo))
	}

	// Bot reclassification band.
	b0, b1 := -1, -1
	for i, m := range months {
		if m >= "2025-03" && m <= "2025-08" {
			if b0 < 0 {
				b0 = i
			}
			b1 = i
		}
	}
	if b0 >= 0 {
		c.Rect(sx(b0), py, math.Max(sx(b1)-sx(b0), 2), ph, band)
		c.Text((sx(b0)+sx(b1))/2, py+6, bandLabel, 6, muted, "middle")
	}
	for v := lo; v <= hi+step/2; v += step {
		c.Line(px, sy(v), px+pw, sy(v), grid, 0.5)
		c.Text(px-4, sy(v), fmt.Sprintf("%.0f", v), 7, muted, "end")
	}
	c.Line(px, sy(100), px+pw, sy(100), muted, 0.6)
	for i, m := range months {
		if strings.HasSuffix(m, "-01") || i == 0 || i == n-1 {
			c.Line(sx(i), py+ph, sx(i), py+ph+3, muted, 0.5)
			c.Text(sx(i), py+ph+9, m, 6.5, muted, "middle")
		}
	}
	for _, l := range lines {
		light := rgb{uint8((int(l.col[0]) + 2*255) / 3), uint8((int(l.col[1]) + 2*255) / 3), uint8((int(l.col[2]) + 2*255) / 3)}
		poly(c, l.raw, sx, sy, light, 0.8)
		poly(c, l.clean, sx, sy, l.col, 1.8)
	}
	// Labels at the right end, nudged apart.
	var ys []float64
	for _, l := range lines {
		yy := sy(lastValid(l.clean))
		for _, o := range ys {
			if math.Abs(yy-o) < 9 {
				yy = o + 9
			}
		}
		ys = append(ys, yy)
		c.Text(px+pw+5, yy, l.lang, 8, l.col, "start")
	}
}

func poly(c canvas, v []float64, sx func(int) float64, sy func(float64) float64, col rgb, w float64) {
	for i := 1; i < len(v); i++ {
		if !math.IsNaN(v[i-1]) && !math.IsNaN(v[i]) {
			c.Line(sx(i-1), sy(v[i-1]), sx(i), sy(v[i]), col, w)
		}
	}
}

func lastValid(v []float64) float64 {
	for i := len(v) - 1; i >= 0; i-- {
		if !math.IsNaN(v[i]) {
			return v[i]
		}
	}
	return 100
}

// svgCanvas renders the chart as a standalone SVG file.
type svgCanvas struct{ b strings.Builder }

func col(c rgb) string { return fmt.Sprintf("rgb(%d,%d,%d)", c[0], c[1], c[2]) }

func (s *svgCanvas) Line(x1, y1, x2, y2 float64, c rgb, w float64) {
	fmt.Fprintf(&s.b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f" stroke-linecap="round"/>`+"\n", x1, y1, x2, y2, col(c), w)
}
func (s *svgCanvas) Rect(x, y, w, h float64, fill rgb) {
	fmt.Fprintf(&s.b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n", x, y, w, h, col(fill))
}
func (s *svgCanvas) Text(x, y float64, t string, size float64, c rgb, anchor string) {
	fmt.Fprintf(&s.b, `<text x="%.1f" y="%.1f" font-size="%.1f" fill="%s" text-anchor="%s" dominant-baseline="middle">%s</text>`+"\n", x, y, size, col(c), anchor, html.EscapeString(t))
}

// ChartSVG returns the chart as an SVG document.
func ChartSVG(r *analysis.Result, title, bandLabel string) string {
	const w, h = 720.0, 320.0
	s := &svgCanvas{}
	s.Rect(0, 0, w, h, rgb{255, 255, 255})
	s.Text(12, 14, title, 10, ink, "start")
	drawChart(s, r, 8, 26, w-16, h-32, bandLabel)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="Noto Sans, DejaVu Sans, Arial, sans-serif">`+"\n%s</svg>\n", w, h, w, h, s.b.String())
}
