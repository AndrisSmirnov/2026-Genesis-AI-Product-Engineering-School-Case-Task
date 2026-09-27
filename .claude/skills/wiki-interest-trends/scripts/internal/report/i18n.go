package report

import (
	"strings"

	"wikitrend/internal/analysis"
)

// Labels are the fixed texts of the one-page report.
type Labels struct {
	Subtitle, Topic, Langs, Period, Months, Source                        string
	ColLang, ColArticle, ColViews, ColYoY, ColTrend, ColVerdict, ColTrust string
	ChartTitle, BandLabel                                                 string
	Conclusion, WhyTrust, NextChecks, Caveats, Assumptions                string
	Verdict, Trust                                                        map[string]string
	AssumptionLines                                                       []string
	Missing                                                               string
	Next                                                                  map[string]string
	Reasons                                                               map[string]string
	Generated                                                             string
}

var labels = map[string]Labels{
	"uk": {
		Subtitle: "Інтерес за переглядами Wikipedia", Topic: "Тема", Langs: "Мови", Period: "Період", Months: "міс.",
		Source:  "Wikimedia Pageviews API, лише люди (agent=user)",
		ColLang: "Мова", ColArticle: "Стаття", ColViews: "Медіана/міс", ColYoY: "Частка, р/р", ColTrend: "Тренд частки/рік (90% ДІ)",
		ColVerdict: "Висновок", ColTrust: "Довіра",
		ChartTitle: "Частка переглядів теми, індекс (перші 12 міс. = 100). Тонка лінія — зі сплесками.",
		BandLabel:  "перекласифікація ботів 2025",
		Conclusion: "Висновок", WhyTrust: "Чому такий рівень довіри", NextChecks: "Що перевірити далі",
		Caveats: "Застереження", Assumptions: "Припущення та обмеження",
		Verdict: map[string]string{"growing": "зростає", "declining": "спадає", "no_clear_trend": "без чіткого тренду"},
		Trust:   map[string]string{"high": "висока", "medium": "середня", "low": "низька"},
		AssumptionLines: []string{
			"Перегляди Wikipedia показують інтерес до теми, а не готовність платити. Мова — не країна: en, es, pt, fr читають у багатьох країнах.",
			"Метрика — частка переглядів теми серед усіх переглядів людьми в мовному розділі (прибирає загальне падіння трафіку Wikipedia); редиректи додано; одноденні сплески прибрано фільтром Хемпеля (±3 дні).",
			"Тренд — нахил Theil–Sen на логарифмі частки; 90% інтервал Сена з поправкою на автокореляцію; напрям — сезонний тест Манна–Кендалла. Пороги довіри відкалібровано на синтетичних рядах.",
		},
		Missing:   "Статті немає",
		Generated: "Згенеровано",
		Next: map[string]string{
			"low_volume":   "Малий обсяг: розширте тему кошиком суміжних статей або оберіть ширшу тему.",
			"spikes":       "Перевірте, що спричинило сплеск у {month} (новини, головна сторінка, соцмережі).",
			"short_window": "Повторіть аналіз на вікні 36 міс.: на коротшому помірне зростання легко пропустити.",
			"missing":      "Статті немає в: {langs}. Ніша може бути незаповненою — перевірте конкурентів цією мовою.",
			"bot_break":    "Зростання збігається з перекласифікацією ботів 2025 — дочекайтеся ще кількох місяців даних.",
			"validate":     "Перевірте попит напряму: пошукові запити, тест лендингу, опитування цільової аудиторії.",
		},
		Reasons: map[string]string{
			"low_volume":        "малий обсяг: медіана {views} перегл./міс (надійно від {min})",
			"direction_clear":   "напрям статистично чіткий (Манн–Кендалл p={p})",
			"direction_unclear": "напрям не відрізняється від шуму (Манн–Кендалл p={p})",
			"effect_small":      "зміна мала або її 90% інтервал включає нуль (поріг {min}%/рік)",
			"spikes":            "результат залежить від сплесків (останній: {month}); без них {clean}%/рік, з ними {raw}%/рік",
			"spike_days":        "кілька днів домінують: топ-3 дні — {share}% річних переглядів",
			"window_sensitive":  "напрям змінюється при зсуві чи подовженні періоду",
			"bot_break":         "тренд залежить від березня–серпня 2025 (перекласифікація ботів)",
			"basket_mixed":      "статті кошика рухаються в різні боки",
			"basket_consistent": "статті кошика рухаються в один бік",
			"robust":            "висновок тримається без сплесків і на інших періодах",
			"crosses_bot_break": "період містить перекласифікацію ботів Wikimedia 2025; частка зменшує, але не прибирає її вплив",
			"article_new":       "переглядів немає до {month}: стаття нова або перейменована",
			"short_window":      "вікно {n} міс. часто пропускає помірне зростання; для твердішої відповіді — 36 міс.",
		},
	},
	"en": {
		Subtitle: "Interest from Wikipedia pageviews", Topic: "Topic", Langs: "Languages", Period: "Period", Months: "mo.",
		Source:  "Wikimedia Pageviews API, humans only (agent=user)",
		ColLang: "Lang", ColArticle: "Article", ColViews: "Median/month", ColYoY: "Share YoY", ColTrend: "Share trend/yr (90% CI)",
		ColVerdict: "Verdict", ColTrust: "Trust",
		ChartTitle: "Topic share of pageviews, index (first 12 months = 100). Thin line — with spikes.",
		BandLabel:  "2025 bot reclassification",
		Conclusion: "Conclusion", WhyTrust: "Why this trust level", NextChecks: "What to check next",
		Caveats: "Caveats", Assumptions: "Assumptions and limitations",
		Verdict: map[string]string{"growing": "growing", "declining": "declining", "no_clear_trend": "no clear trend"},
		Trust:   map[string]string{"high": "high", "medium": "medium", "low": "low"},
		AssumptionLines: []string{
			"Wikipedia pageviews show interest in a topic, not willingness to pay. Language is not country: en, es, pt, fr are read in many countries.",
			"Metric: the topic's share of all human pageviews of the language edition (removes Wikipedia's overall traffic decline); redirects included; one-day spikes removed with a Hampel filter (±3 days).",
			"Trend: Theil–Sen slope of log share; 90% Sen interval widened for autocorrelation; direction: seasonal Mann–Kendall test. Trust thresholds calibrated on synthetic series.",
		},
		Missing:   "No article",
		Generated: "Generated",
		Next: map[string]string{
			"low_volume":   "Low volume: widen the topic with a basket of related articles or pick a broader topic.",
			"spikes":       "Check what caused the spike in {month} (news, main page, social media).",
			"short_window": "Rerun with a 36-month window: shorter windows easily miss moderate growth.",
			"missing":      "No article in: {langs}. The niche may be unfilled — check competitors in that language.",
			"bot_break":    "Growth coincides with the 2025 bot reclassification — wait for a few more months of data.",
			"validate":     "Validate demand directly: search queries, a landing-page test, a survey of the target audience.",
		},
		Reasons: map[string]string{},
	},
}

func lbl(lang string) Labels {
	if l, ok := labels[lang]; ok {
		return l
	}
	return labels["en"]
}

// reasonText localises a reason; English falls back to the analysis text.
func reasonText(L Labels, r analysis.Reason) string {
	t, ok := L.Reasons[r.Code]
	if !ok {
		return r.Text
	}
	for k, v := range r.Params {
		t = strings.ReplaceAll(t, "{"+k+"}", v)
	}
	return t
}

// ChartTitle and BandLabel are used for the standalone SVG chart.
func ChartTitle(lang string) string { return lbl(lang).ChartTitle }

// BandLabel labels the 2025 bot reclassification band.
func BandLabel(lang string) string { return lbl(lang).BandLabel }

// Localize returns placeholder values with verdict and trust translated into
// the report language, so "{{uk.trust}}" reads "середня" in a Ukrainian report.
func Localize(values map[string]string, lang string) map[string]string {
	L := lbl(lang)
	res := make(map[string]string, len(values))
	for k, v := range values {
		res[k] = v
		switch {
		case strings.HasSuffix(k, ".verdict"):
			if t, ok := L.Verdict[v]; ok {
				res[k] = t
			}
		case strings.HasSuffix(k, ".trust"):
			if t, ok := L.Trust[v]; ok {
				res[k] = t
			}
		}
	}
	return res
}
