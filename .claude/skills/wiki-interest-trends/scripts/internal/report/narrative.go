package report

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"wikitrend/internal/out"
)

// Limits keep the narrative inside the one-page layout.
const (
	MaxHeadline = 140
	MaxBody     = 1100
)

var placeholderRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.\-]+)\s*\}\}`)

// Narrative is the model's text after validation and substitution.
type Narrative struct {
	Headline string
	Body     string
}

// Render validates the model's text and substitutes placeholders.
//
// The model may not type digits: every number must come from a placeholder,
// so it is impossible to put an invented or mistyped number into the report.
func Render(text string, values map[string]string, next string) (Narrative, error) {
	var unknown []string
	stripped := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		k := placeholderRe.FindStringSubmatch(m)[1]
		if _, ok := values[k]; !ok {
			unknown = append(unknown, k)
		}
		return " "
	})
	if len(unknown) > 0 {
		e := out.Errf("UNKNOWN_PLACEHOLDER", next, "unknown placeholders: %s", strings.Join(unknown, ", "))
		e.Details = map[string]any{"available": keys(values)}
		return Narrative{}, e
	}
	if bad := digitSnippets(stripped); len(bad) > 0 {
		e := out.Errf("RAW_NUMBER", next,
			"the narrative contains digits outside placeholders; replace every number with a {{placeholder}} (e.g. {{uk.trend}}) or remove it")
		e.Details = map[string]any{"snippets": bad}
		return Narrative{}, e
	}
	rendered := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		return values[placeholderRe.FindStringSubmatch(m)[1]]
	})
	// Values already carry "%": "{{uk.trend}}%" must not print "+12.4%%".
	rendered = strings.ReplaceAll(rendered, "%%", "%")
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(rendered, "\r\n", "\n"), "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	var n Narrative
	i := 0
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			n.Headline = strings.TrimSpace(strings.TrimLeft(lines[i], "# "))
			i++
			break
		}
	}
	n.Body = strings.TrimSpace(strings.Join(lines[i:], "\n"))
	n.Body = strings.NewReplacer("**", "", "__", "").Replace(n.Body)
	if n.Headline == "" || n.Body == "" {
		return n, out.Errf("BAD_NARRATIVE", next, "the narrative needs a first line (headline) and at least one paragraph after it")
	}
	if l := utf8.RuneCountInString(n.Headline); l > MaxHeadline {
		return n, out.Errf("NARRATIVE_TOO_LONG", next, "headline is %d characters after substitution; keep it under %d", l, MaxHeadline)
	}
	if l := utf8.RuneCountInString(n.Body); l > MaxBody {
		return n, out.Errf("NARRATIVE_TOO_LONG", next, "body is %d characters after substitution; shorten it to under %d", l, MaxBody)
	}
	return n, nil
}

func digitSnippets(s string) []string {
	r := []rune(s)
	var res []string
	for i := 0; i < len(r); i++ {
		if unicode.IsDigit(r[i]) {
			lo, hi := max(0, i-15), min(len(r), i+15)
			res = append(res, "…"+strings.TrimSpace(string(r[lo:hi]))+"…")
			for i < len(r) && (unicode.IsDigit(r[i]) || r[i] == '.' || r[i] == ',' || r[i] == '%') {
				i++
			}
			if len(res) == 5 {
				break
			}
		}
	}
	return res
}

func keys(m map[string]string) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}

// DetectLang picks the report label language from the narrative itself:
// mostly Cyrillic letters → "uk", otherwise "en".
func DetectLang(text string) string {
	text = placeholderRe.ReplaceAllString(text, " ")
	var cyr, lat int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case unicode.IsLetter(r):
			lat++
		}
	}
	if cyr > lat {
		return "uk"
	}
	return "en"
}
