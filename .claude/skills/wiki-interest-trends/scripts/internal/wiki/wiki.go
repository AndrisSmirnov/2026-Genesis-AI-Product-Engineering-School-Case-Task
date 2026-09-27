// Package wiki wraps the three upstream APIs the skill uses:
// Wikidata (topic → entity → sitelinks), MediaWiki (redirects) and the
// Wikimedia Analytics API (pageviews).
package wiki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"wikitrend/internal/httpx"
	"wikitrend/internal/out"
)

const (
	wikidataAPI = "https://www.wikidata.org/w/api.php"
	metricsAPI  = "https://wikimedia.org/api/rest_v1/metrics/pageviews"
	metaTTL     = 7 * 24 * time.Hour
)

// Client groups the upstream calls.
type Client struct{ H *httpx.Client }

// Candidate is a Wikidata entity that may match the user's topic.
type Candidate struct {
	QID         string            `json:"qid"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Sitelinks   map[string]string `json:"sitelinks"`   // lang -> article title (requested langs only)
	TotalWikis  int               `json:"total_wikis"` // how many Wikipedias have an article (notability)
	Disambig    bool              `json:"-"`
}

// Hit is one search result. Exact means the query equals the entity's label
// or one of its aliases (case-insensitive) — fuzzy matches rank below.
type Hit struct {
	QID   string
	Exact bool
}

// Search finds up to limit entities matching topic. It searches in each given
// UI language (the topic may be written in Ukrainian, Polish, English...) and
// keeps the first-seen order.
func (c *Client) Search(topic string, searchLangs []string, limit int) ([]Hit, error) {
	idx := map[string]int{}
	var hits []Hit
	for _, lang := range searchLangs {
		q := url.Values{
			"action": {"wbsearchentities"}, "search": {topic}, "language": {lang},
			"uselang": {lang}, "type": {"item"}, "limit": {fmt.Sprint(limit)}, "format": {"json"},
		}
		_, body, err := c.H.Get(wikidataAPI+"?"+q.Encode(), metaTTL)
		if err != nil {
			return nil, err
		}
		var r struct {
			Search []struct {
				ID    string `json:"id"`
				Match struct {
					Text string `json:"text"`
				} `json:"match"`
			} `json:"search"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("wikidata search: %w", err)
		}
		for _, s := range r.Search {
			exact := strings.EqualFold(strings.TrimSpace(s.Match.Text), strings.TrimSpace(topic))
			if i, ok := idx[s.ID]; ok {
				hits[i].Exact = hits[i].Exact || exact
				continue
			}
			idx[s.ID] = len(hits)
			hits = append(hits, Hit{QID: s.ID, Exact: exact})
		}
	}
	return hits, nil
}

// Entities loads labels, descriptions and sitelinks for the given QIDs.
func (c *Client) Entities(qids []string, langs []string, uiLang string) ([]Candidate, error) {
	if len(qids) == 0 {
		return nil, nil
	}
	q := url.Values{
		"action": {"wbgetentities"}, "ids": {strings.Join(qids, "|")},
		"props":     {"labels|descriptions|sitelinks"},
		"languages": {uiLang + "|en"}, "format": {"json"},
	}
	_, body, err := c.H.Get(wikidataAPI+"?"+q.Encode(), metaTTL)
	if err != nil {
		return nil, err
	}
	type lv struct {
		Value string `json:"value"`
	}
	var r struct {
		Entities map[string]struct {
			Missing      *string       `json:"missing"`
			Labels       map[string]lv `json:"labels"`
			Descriptions map[string]lv `json:"descriptions"`
			Sitelinks    map[string]struct {
				Title string `json:"title"`
			} `json:"sitelinks"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("wikidata entities: %w", err)
	}
	pick := func(m map[string]lv) string {
		if v, ok := m[uiLang]; ok {
			return v.Value
		}
		return m["en"].Value
	}
	var res []Candidate
	for _, id := range qids {
		e, ok := r.Entities[id]
		if !ok || e.Missing != nil {
			continue
		}
		c := Candidate{QID: id, Label: pick(e.Labels), Description: pick(e.Descriptions), Sitelinks: map[string]string{}}
		en := strings.ToLower(e.Descriptions["en"].Value)
		c.Disambig = strings.Contains(en, "disambiguation page") || strings.Contains(en, "wikimedia list")
		for site := range e.Sitelinks {
			if strings.HasSuffix(site, "wiki") && !strings.Contains(site, "commons") && !strings.Contains(site, "species") && !strings.Contains(site, "meta") {
				c.TotalWikis++
			}
		}
		for _, l := range langs {
			if s, ok := e.Sitelinks[l+"wiki"]; ok {
				c.Sitelinks[l] = s.Title
			}
		}
		res = append(res, c)
	}
	return res, nil
}

// Redirects returns up to limit main-namespace redirects to title.
func (c *Client) Redirects(lang, title string, limit int) ([]string, error) {
	q := url.Values{
		"action": {"query"}, "titles": {title}, "prop": {"redirects"},
		"rdnamespace": {"0"}, "rdlimit": {fmt.Sprint(limit)},
		"format": {"json"}, "formatversion": {"2"},
	}
	_, body, err := c.H.Get(fmt.Sprintf("https://%s.wikipedia.org/w/api.php?%s", lang, q.Encode()), metaTTL)
	if err != nil {
		return nil, err
	}
	var r struct {
		Query struct {
			Pages []struct {
				Redirects []struct {
					Title string `json:"title"`
				} `json:"redirects"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("redirects %s:%s: %w", lang, title, err)
	}
	var res []string
	for _, p := range r.Query.Pages {
		for _, rd := range p.Redirects {
			res = append(res, rd.Title)
		}
	}
	sort.Strings(res)
	return res, nil
}

type pvResp struct {
	Items []struct {
		Timestamp string `json:"timestamp"`
		Views     int64  `json:"views"`
	} `json:"items"`
}

// ttlFor: data for fully finished days never changes, so it is cached forever.
func ttlFor(end time.Time) time.Duration {
	if time.Since(end) > 72*time.Hour {
		return 0
	}
	return 24 * time.Hour
}

// ArticleDaily returns daily user views of one article, keyed by "YYYY-MM-DD".
// Days without data are absent; 404 means zero views (API cannot tell apart).
func (c *Client) ArticleDaily(lang, access, title string, from, to time.Time) (map[string]int64, error) {
	t := url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	u := fmt.Sprintf("%s/per-article/%s.wikipedia/%s/user/%s/daily/%s/%s",
		metricsAPI, lang, access, t, from.Format("20060102"), to.Format("20060102"))
	return c.series(u, ttlFor(to), "2006010200", "2006-01-02")
}

// ProjectMonthly returns monthly user views of a whole language edition, keyed by "YYYY-MM".
func (c *Client) ProjectMonthly(lang, access string, from, to time.Time) (map[string]int64, error) {
	u := fmt.Sprintf("%s/aggregate/%s.wikipedia/%s/user/monthly/%s00/%s00",
		metricsAPI, lang, access, from.Format("20060102"), to.Format("20060102"))
	m, err := c.series(u, ttlFor(to), "2006010200", "2006-01")
	if err == nil && len(m) == 0 {
		return nil, out.Errf("NO_DATA", "Check the language code (e.g. uk, pl, cs, en).", "no aggregate pageviews for %s.wikipedia", lang)
	}
	return m, err
}

func (c *Client) series(u string, ttl time.Duration, inLayout, outLayout string) (map[string]int64, error) {
	status, body, err := c.H.Get(u, ttl)
	if err != nil {
		return nil, err
	}
	res := map[string]int64{}
	if status == 404 {
		return res, nil
	}
	var r pvResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("pageviews %s: %w", u, err)
	}
	for _, it := range r.Items {
		ts, err := time.Parse(inLayout, it.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("pageviews timestamp %q: %w", it.Timestamp, err)
		}
		res[ts.Format(outLayout)] += it.Views
	}
	return res, nil
}
