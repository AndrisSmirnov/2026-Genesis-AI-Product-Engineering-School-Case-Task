# Wikipedia-тренди на Go + Claude Code: дослідницька основа для навички й процесу розробки (вересень 2026)

Так, Go підходить для цієї навички. Але три умови обов'язкові: Go-модуль запускається через `go run` з консервативним рядком `go` у go.mod; кожен HTTP-запит до Wikimedia має явний User-Agent, бо з 2026 року діють жорсткі ліміти; усі числа, графіки й PDF рахує код, а модель пише лише текст із плейсхолдерами. Тестувати на безкоштовних моделях варто не через Claude Code, а через OpenCode або власний мінімальний Go-харнес.

## TL;DR

- **Архітектура:** Go-модуль у `scripts/` навички, 4–5 високорівневих команд CLI з компактним JSON. Метрика — частка переглядів теми від усіх *людських* (`agent=user`) переглядів мовного розділу, бо сам знаменник з 2025 року падає. Рівень довіри має три щаблі й визначається 5–7 незалежними перевірками. Theil–Sen і Mann–Kendall реалізуєш сам і звіряєш з еталонами Python/R.
- **Головні зовнішні ризики:** нові ліміти Wikimedia API (2026, сторінка mediawiki.org «Wikimedia APIs/Rate limits»): 10 запитів/хв без ідентифікації, 200 запитів/хв з коректним User-Agent; злам ряду через ретроспективну перекласифікацію ботів у березні–серпні 2025 (людські перегляди −8% р/р, за блогом WMF Diff від 17.10.2025); Claude API code execution не має мережі; Claude Code через OpenRouter гарантовано працює лише з Anthropic-моделями, а безкоштовні моделі, за довідковим центром OpenRouter, обмежені 20 запитами/хв і 50 (або 1000) запитами на день.
- **Процес:** eval-first. 12–15 сценаріїв; програмні перевірки JSON і PDF плюс LLM-суддя з рубрикою; кожен сценарій ≥3 рази на Haiku 4.5 у headless-режимі. Хуки: `gofmt`/`goimports` після редагування, `go vet` + `go test` на Stop. Субагенти: рев'юер Go, верифікатор статистики, «дешевий тестувальник» на Haiku з підвантаженою навичкою.

## 1. Резюме: 12 ключових висновків

1. **Специфікація Agent Skills дозволяє Go, але нічого не гарантує:** «Supported languages depend on the agent implementation. Common options include Python, Bash, and JavaScript». Вимогу Go декларуй у `compatibility` (≤500 символів) і дай у SKILL.md одну точну команду запуску.\[1\] Claude Code поле `compatibility` приймає, але «doesn't act on it», тож перевірку середовища робить сама навичка командою `doctor`.\[2\]
2. **Go є в хмарних сесіях Claude Code** («Go with module support»), а `proxy.golang.org` і `sum.golang.org` там дозволені за замовчуванням.\[3\] **Claude API code execution** «has no internet access» — навичка з Wikimedia API там принципово не працює; напиши це в `compatibility`.\[4\]\[5\]
3. **Wikimedia змінилася в 2025–2026.** Сторінка mediawiki.org «Wikimedia APIs/Rate limits» описує глобальні ліміти API з березня–квітня 2026: 10 запитів/хв для неідентифікованих, 200 запитів/хв для ботів із коректним User-Agent («User-Agent only») і 2000 запитів/хв для досвідчених автентифікованих редакторів; мета — зменшити частку неідентифікованих автоматизованих запитів до API («about 33% at the end of 2025»), а самі ліміти «new in 2026 and are subject to experimentation and change». З серпня 2025 Wikimedia поступово блокує «library default» User-Agent'и (Phabricator T400119) — стандартний `Go-http-client/1.1` саме такий.\[6\]
4. **Базовий URL не змінився**: `https://wikimedia.org/api/rest_v1/metrics/...`. \[7\] Змінилися документація (doc.wikimedia.org/generated-data-platform/aqs/analytics-api), політика доступу й ліміти. Дані є з 1 липня 2015.\[8\] 2025-12-19 додано ендпоінти «Pageviews per editor».\[9\]
5. **Ряд переглядів має структурний злам у 2025.** За постом Marshall Miller у блозі WMF Diff «New User Trends on Wikipedia» (17.10.2025), «around May 2025» Wikimedia побачила незвично багато нібито людського трафіку, «mostly originating from Brazil», і з'ясувала, що в травні–червні це були боти, «built to evade detection»; після оновлення детекції й перекласифікації березня–серпня 2025 людські перегляди виявилися нижчими на «roughly 8% as compared to the same months in 2024». Отже, нормалізація на `agent=user`-агрегат обов'язкова, а порівняння «до/після травня 2025» позначається як ризикове.
6. **Редиректи не додаються до цільової статті**: «Redirects… aren't counted as views of the actual page».\[10\] Їх треба сумувати вручну (як Redirect Views).\[11\] 404 від per-article означає або нуль переглядів, або ще не завантажені дані; нулі в рядах пропускаються.\[12\]
7. **H3 підтверджується з уточненнями:** основний рядок — місячна частка теми в людських переглядах розділу; зростання — YoY за ковзною 12-місячною сумою, CAGR як похідна від 12-місячних сум, Theil–Sen-нахил на log(частки).
8. **Довіра — три рівні**, які визначає не p-value, а сукупність перевірок: напрям (сезонний MK), практична величина, стійкість до сплесків (Hampel/MAD), обсяг, чутливість до вікна, узгодженість кошика, відсутність зламу. Пороги — гіпотези для калібрування на синтетиці.
9. **Go-бібліотеки (вересень 2026):** PDF — `signintech/gopdf` (v0.38.1, 12.09.2026, MIT); `go-pdf/fpdf` архівовано 4.03.2025, `jung-kurt/gofpdf` — 13.11.2021.\[13\]\[14\]\[15\]\[16\] Графіки — `gonum.org/v1/plot` (v0.17.0); `wcharczuk/go-chart` архівовано 23.08.2024.\[17\]\[18\] Кеш — `bbolt` (v1.5.0, 3.06.2026) або `modernc.org/sqlite` (v1.59.0 від 15.09.2026 і новіші).\[19\]\[20\] У gonum є лише кореляція Kendall; Mann–Kendall і Theil–Sen пишеш сам.\[21\]
10. **Перший `go run` — ризик для дешевої моделі:** агент може вирішити, що команда «зависла». Мінімум залежностей (у v1 `bbolt` або JSON-файли замість важкого `modernc.org/sqlite`), `doctor --warmup` першим кроком, у SKILL.md — «перший запуск може тривати до ~1–2 хв» (гіпотеза, заміряй).
11. **Стенд:** основний — Claude Code з `--model haiku` у headless-режимі. OpenRouter офіційно підтримує Claude Code (`ANTHROPIC_BASE_URL=https://openrouter.ai/api`), але «only guaranteed to work with the Anthropic first-party provider».\[22\]\[23\] Тому free-моделі ганяй через OpenCode (читає `.claude/skills/`) або власний ~300-рядковий агентний цикл на Go з повним контролем логів, токенів і повторів.\[24\]
12. **OpenRouter free:** за довідковим центром OpenRouter (стаття «OpenRouter Rate Limits»), «Free usage: 50 requests/day, 20 requests/minute», а після покупки кредитів на суму понад $10 — «1000 requests/day, 20 requests/minute», і вищий ліміт зберігається, навіть коли баланс потім падає нижче $10; часті 429 від upstream-провайдерів навіть при невичерпаній квоті; за тестом klymentiev.com від 11.09.2026, у каталозі 19 текстових моделей із суфіксом `:free` плюс шість безкоштовних моделей для ембедінгів, ранжування й синтезу мовлення, і склад змінюється. Сторінка документації `:free` каже лише, що free-варіанти «may have different rate limits or availability».

## 2. Вердикт по H1–H7

| Гіпотеза | Вердикт | Чому |
|---|---|---|
| **H1** Go-модуль, `go run`, 3–5 команд, компактний JSON | **Підтверджено, з поправками** | `go run` компілює в кеш збірки й не лишає бінарника в репо. Поправки: (1) `toolchain` — не гарантія версії, а тригер автозавантаження; при `GOTOOLCHAIN=local` (так в офіційних Docker-образах Go) новіший рядок дає «go.mod requires go >= …».\[25\]\[26\] Рядок `go` — мінімально потрібний, `toolchain` не додавай. (2) Запуск через `go -C <skill>/scripts run ./cmd/wikitrend …`, щоб не залежати від cwd. (3) Команда `doctor` п'ятою. |
| **H2** Wikidata → sitelinks + редиректи; кошик опційно | **Підтверджено** | QID дає однозначність і всі мови; редиректи обов'язкові. Кошик — лише для широких тем, фіксується в spec-файлі (QID, заголовки, дата) для відтворюваності. |
| **H3** Частка, YoY, CAGR, робастний нахил | **Змінити деталі** | Знаменник — `aggregate/{project}/all-access/user/monthly`, не all-agents. YoY за 12-місячними сумами; CAGR з кінцевих точок чутливий до сплесків. Частка — основна метрика, абсолюти — довідкові, бо падіння Wikipedia на ~8% р/р маскує тренд. |
| **H4** Рівень довіри; статистика на Go з еталонами | **Підтверджено** | MK, сезонний MK, Theil–Sen, Hampel, block bootstrap — по ~50–150 рядків. Еталони: `pymannkendall`, `scipy.stats.theilslopes`, R `trend`. У gonum лише `stat.Kendall` (Tau-a). Імена еталонних функцій — із загальних знань, перевір версії. |
| **H5** Спека JSON/YAML + кеш без cgo | **Підтверджено, спростити v1** | Спека робить уточнення патчем одного поля. Для v1 — `bbolt` або `cache/<sha256(url)>.json` з TTL; `modernc.org/sqlite` — у v2 (швидша перша збірка). |
| **H6** Однострінковий PDF з TTF; графіки на Go; числа від коду; валідатор | **Підтверджено, змінити бібліотеку** | `gopdf` (активна) замість архівного fpdf; шрифт DejaVu/Noto через `go:embed`. Одна сторінка — фіксований макет зі слотами + тест кількості сторінок. Надійніше **заборонити моделі писати числа**: плейсхолдери `{{pl.yoy}}` + валідатор, що відхиляє цифри поза ними. |
| **H7** Haiku 4.5 headless + одна модель OpenRouter | **Змінити** | Haiku у Claude Code — так. Не-Anthropic моделі в Claude Code ненадійні (застереження OpenRouter) — бери OpenCode або свій Go-харнес. Ліміт 50 запитів/день: сценарій = 10–20 запитів, тож 15×3 без кредитів не вміститься. |

## 3. Таблиця рішень

| Рішення | Варіанти | Рекомендація | Обґрунтування | Ризики |
|---|---|---|---|---|
| Мова коду | Go / Python+uv / гібрид | Go; Python лише для генерації еталонних фікстур поза навичкою | Основна мова, легше захищати, один `go run` | Go відсутній у середовищі; довгий перший запуск |
| Запуск і версія | `go run` / build у temp / vendor; `go`+`toolchain` чи лише `go` | `go -C scripts run ./cmd/wikitrend`, лише рядок `go`, go.sum без vendor (`make vendor` опційно) | Без бінарників, сумісно з `GOTOOLCHAIN=local` | Залежність від GOPROXY; немає найновіших фіч мови |
| Дані й HTTP | REST / дампи / Toolforge | REST v1 + `net/http` + `x/time/rate` (≈2–3 запити/с) + backoff за `Retry-After` | Достатньо для 1–50 статей × кілька мов | 429, ліміт 200/хв |
| Тема → статті | Пошук назви / Wikidata / категорії | `wbsearchentities` → `wbgetentities` (sitelinks) + MediaWiki redirects | Однозначність QID | Неоднозначні й відсутні статті |
| Метрика й тренд | Абсолюти / частка; OLS / Theil–Sen / STL | Частка від `user`-агрегату; Theil–Sen на log + сезонний MK | Прибирає загальне падіння; робастність | Шум малих розділів; автокореляція |
| Довіра | p-value / бали / рівні | 3 рівні з причинами | Зрозуміло фаундеру, без хибної точності | Потрібне калібрування |
| Кеш | файли / bbolt / sqlite | v1 файли або bbolt; v2 sqlite | Швидкий перший запуск | Міграція пізніше |
| Графіки / PDF | gonum/plot, go-chart; gopdf, maroto, fpdf | gonum/plot → PNG; gopdf | Активні; інші архівні | Ручний макет |
| Числа в тексті | Валідатор / плейсхолдери | Плейсхолдери + заборона сирих цифр | Модель фізично не вигадає число | Менш «природний» текст |
| Стенд free | CC+OpenRouter / OpenCode / свій харнес | OpenCode + Go-харнес | Гарантія CC лише для Anthropic | Різна поведінка агентів |

## 4. Детальні знахідки

### A. Дані й методологія

**A1. Wikimedia Analytics API (вересень 2026)**

- **Ендпоінти** (база `https://wikimedia.org/api/rest_v1/metrics/`, дані CC0):\[7\]\[27\]
  - per-article: `/pageviews/per-article/{project}/{access}/{agent}/{article}/{daily|monthly}/{start}/{end}` (hourly, за стороннім джерелом, дає 400);\[28\]
  - aggregate: `/pageviews/aggregate/{project}/{access}/{agent}/{hourly|daily|monthly}/{start}/{end}`; `all-projects` приймає лише aggregate;\[7\]\[29\]\[30\]
  - top: `/pageviews/top/{project}/{access}/{year}/{month}/{day|all-days}`, до 1000 статей;\[30\]
  - країни: `/pageviews/top-by-country/{project}/{access}/{year}/{month}` — помісячно, бакети (`"1000000-9999999"`) + `rank`, країни з ≤100 переглядами приховано; `top-per-country/{country}/…` підтверджено лише вторинно;\[31\]\[32\]
  - unique devices: `/unique-devices/{project}/{all-sites|desktop-site|mobile-site}/{daily|monthly}/{start}/{end}`;\[32\]
  - legacy pagecounts (грудень 2007 — липень 2016) включають ботів — не змішувати з pageviews.\[33\]
- **Параметри:** access = `all-access|desktop|mobile-app|mobile-web`; agent = `all-agents|user|spider|automated` (`automated` з 2020-04-29 — евристичні боти; `spider` — самоідентифіковані).\[9\]\[10\]
- **Затримка:** «usually done within a few hours, but can take 24 hours or more».\[12\] Практично: поточний неповний місяць виключай. Документованого максимуму діапазону не знайдено.
- **Помилки:** 404 — нуль або ще не завантажено (API не розрізняє); 429 — тротлінг (кешовані відповіді не тротляться); 400 — невалідні параметри.\[12\]
- **Доступ:** User-Agent обов'язковий, формат `<client>/<version> (<contact>) <library>/<version>`; access policy радить послідовні запити.\[27\] Ліміти 2026 за сторінкою mediawiki.org «Wikimedia APIs/Rate limits» (експериментальні): 10 запитів/хв для неідентифікованих, 200 — для «User-Agent only» з коректним UA і для нових автентифікованих користувачів, 2000 — для досвідчених автентифікованих редакторів. Лист Jonathan Tweed (WMF) «New global API rate limits» у wikitech-l від 2 березня 2026: «In early March, we will apply low limits to anonymous API requests… In early April, higher limits will be applied to identified traffic»; автентифіковані запити від групи 'bot' і з Toolforge/WMCS під ці ліміти не підпадають. Що вони покривають саме `/metrics`, прямо не підтверджено, але сторінка каже «across all… REST APIs».

Типовий запит «2 мови × 5 статей + редиректи + 2 агрегати» — ~20–40 викликів: 10–20 с на 2 запитах/с, 0 с з кешу.

**A2. Пастки даних**

| Пастка | Обробка |
|---|---|
| Редиректи не зараховуються цілі\[10\] | MediaWiki `prop=redirects`, сумувати ті, що мають ≥1% трафіку цілі |
| Перейменування розриває історію | Об'єднувати ряди старої й нової назв через редиректи |
| Боти + перекласифікація 2025 | Завжди `agent=user`; прапорець «ряд перетинає травень 2025»; порівнювати частку |
| Сплески (новини, головна, Doodle, соцмережі) | Hampel на денних даних, частка топ-3 днів, перерахунок тренду без сплесків |
| Desktop → mobile | `all-access` у чисельнику й знаменнику; решта — діагностика |
| Загальне падіння трафіку: −8% р/р (блог WMF Diff, Marshall Miller, 17.10.2025); −10.6% en.wikipedia березень 2025 → березень 2026 (неофіційний GitHub monperrus) | Нормалізація на агрегат розділу + примітка у звіті про AI-відповіді й соцмережі |
| Зростання окремих розділів (напр., uk після 2022 — не перевірено) | Частка компенсує; окремо показувати тренд знаменника |
| 404 / пропущені нулі\[12\] | Заповнювати нулями, обрізати кінець до гарантовано завантаженого дня |

**Готові інструменти:** Pageviews Analysis (pageviews.wmcloud.org, MIT): Pageviews, Langviews, Redirect Views, Massviews, Topviews, Siteviews — використовуй як ручний оракул для 3–5 кейсів.\[34\]

**A3. Тема → статті**

- **Однозначна сутність:** `wbsearchentities` → кандидати з описами на підтвердження моделі/користувача → `wbgetentities&props=sitelinks`. Відсутній sitelink → `missing: ["cs"]` у JSON (це теж інсайт: низький інтерес або незаповнена ніша).
- **Широка тема** («вивчення англійської»): кошик кількох QID (English language, ESL/EFL, IELTS, TOEFL…), пропонує модель, підтверджує користувач, фіксує `spec` з датою розв'язання.
- **Категорії/посилання** — лише джерело кандидатів: глибина категорій різниться між мовами.
- Параметри Wikidata API — із загальних знань, окремо не перевірені. Ліміти 2026 діють і на Wikidata (кейс OpenRefine #7731: 429 через неналежний UA).\[35\]

**A4. Методика «чи зростає і наскільки довіряти»**

1. Денні ряди → нулі → Hampel (±3 дні, поріг 3·1.4826·MAD) → прапорці сплесків.
2. Місячні суми (сирі й очищені) → частка = тема / агрегат(`user`).
3. YoY за 12-місячними сумами; CAGR = (S_last12 / S_first12)^(1/років) − 1; Theil–Sen log(частки) → exp(12·slope) − 1 «% за рік».
4. MK (≥24 міс — сезонний з періодом 12); автокореляція — Hamed–Rao або block bootstrap.
5. Bootstrap: блоки по 3 місяці, 1000 повторів → 90% інтервал.
6. Чутливість: вікна 24/36 міс, зсув старту ±3 міс, з/без сплесків.

Сам реалізуй медіану/MAD, Hampel, Theil–Sen (O(n²) для n ≤ 200 — дрібниця), MK/SMK (поправка на зв'язки, p через `distuv.Normal`), bootstrap. STL у v1 не роби — сезонний MK + YoY і так нейтралізують сезонність. Еталони генеруй одноразовим Python-скриптом у `dev/` і комить JSON-фікстури.

**A5. Мова ≠ ринок (P1)**

`top-by-country` дає лише ранжування країн (бакети); для en/es/pt/fr висновок про конкретну країну майже неможливий.\[31\] Unique devices оцінюють розмір аудиторії розділу.\[36\] У звіті завжди: інтерес ≠ платоспроможність; Wikipedia історично отримувала «almost 90%» відвідувачів з Google search (WMF у переказі Search Engine Journal), тож зміни пошуку (AI Overviews) зсувають ряди незалежно від інтересу.\[37\] Сигнали для v3+: правки/редактори статті (той самий API), наявність і довжина статті, Google Trends (офіційного вільного API немає — перевір).

### B. Дизайн навички на Go

**B1. Специфікація.** `name` ≤64 символи (a-z0-9, дефіси, без `--`, = назва директорії); `description` ≤1024; опційні `license`, `compatibility` (≤500), `metadata`, `allowed-tools` (експериментальне). Progressive disclosure: метадані ~100 токенів → тіло <5000 токенів / <500 рядків → ресурси; посилання на один рівень. Валідація: `skills-ref validate`.\[1\] Anthropic забороняє в name XML і слова «anthropic», «claude».\[38\]\[39\]

Підтримка: клієнти, що документують SKILL.md, — Claude Code, Claude apps/API, Codex, Cursor, Copilot/VS Code, Gemini CLI, OpenCode, goose, Amp; шоукейс agentskills.io на 20.08.2026 — 46 продуктів (skillsboard.sh).\[40\] Відмінності: Gemini CLI просить згоду перед активацією; OpenCode має allowlist полів і дозволи allow/deny/ask; Claude Code розширює стандарт (`disable-model-invocation`, `context: fork`, `!`command``).\[2\]\[40\] Шляхи: `.claude/skills/`; `.agents/skills/` у Codex, Gemini, Copilot, OpenCode; OpenCode читає й `.claude/skills/`.\[24\]\[41\]\[42\]

Рекомендований `compatibility`: `Requires Go 1.24+ on PATH (uses 'go run'; modules from proxy.golang.org on first run) and HTTPS access to wikimedia.org and wikidata.org. Not for sandboxes without network (e.g. Claude API code execution).` (1.24 — припущення.)

**B2. Best practices.** Description від третьої особи з тригерами; skill-creator радить трохи «напористий» опис, бо Claude недотригерює; у Claude Code description + `when_to_use` обрізаються до 1536 символів.\[43\]\[44\] «What works perfectly for Opus might need more detail for Haiku».\[45\] skill-creator оптимізує description: 20 запитів should/should-not, 60/40 train/test, 3 прогони на запит, до 5 ітерацій, покращення на 5 з 6 навичок Anthropic; незалежний досвід (dev.to) показує, що він не завжди перевершує ручний опис.\[46\]\[47\]\[48\] Для маленької моделі: один золотий шлях (5–7 кроків), розгалуження в `references/`, точні команди для копіювання, `next_step` у кожному виході.

**B3. CLI для слабкої моделі.** 4–5 команд замість 15; stdout ≤1–2 КБ (`status`, `summary`, `warnings`, `next_step`, шляхи файлів), сирі ряди лише у `runs/<id>/data.json`. Помилки з `code`, `hint`, `next_step` і ненульовим exit code. `run_id` = хеш канонізованої спеки (ідемпотентність). Уточнення: `analyze --from <id> --set window.months=36` створює спеку-нащадка з `parent_run_id`.

**B4. Go без бінарників.** `GOTOOLCHAIN=auto` (дефолт) завантажує тулчейн як модуль `golang.org/toolchain`; `local` вимикає завантаження (офіційні Docker-образи, `actions/setup-go`) — тому рядок `go` ≤ реальної версії середовища.\[25\]\[26\]\[49\] Якщо Go немає, `doctor` повертає `GO_MISSING` з підказкою (go.dev/dl, `winget install GoLang.Go`); жодних запасних бінарників і запасного Python-шляху (подвоєння коду й тестів). Windows: `go -C` у Git Bash працює; уникай симлінків для навички. `CGO_ENABLED=0` у Makefile і тестах.

**B5. Бібліотеки (вересень 2026)**

| Задача | Бібліотека | Статус | Ліцензія | Рішення |
|---|---|---|---|---|
| HTTP + ліміт | `net/http` + `golang.org/x/time/rate` | стандарт | BSD | Так |
| Статистика | `gonum/stat`, `stat/distuv` | активна | BSD-3 | Базові функції + нормальний розподіл; MK/Theil–Sen свої |
| Графіки | `gonum.org/v1/plot` | v0.17.0\[18\] | BSD-3 | Так (vgimg → PNG, vgsvg → SVG) |
| Графіки | `wcharczuk/go-chart/v2` | архів з 23.08.2024\[17\] | MIT | Ні |
| PDF | `signintech/gopdf` | v0.38.1, 12.09.2026 | MIT\[15\] | Так |
| PDF | `go-pdf/fpdf` | архів з 04.03.2025 | MIT\[13\] | Ні |
| PDF | `johnfercher/maroto/v2` | активна, але на архівному gofpdf (за конкурентом gpdf.dev — упереджено) | MIT | Ні для v1 |\[16\]
| Кеш | `go.etcd.io/bbolt` | v1.5.0, 03.06.2026 | MIT\[19\] | v1 |
| SQL | `modernc.org/sqlite` | v1.59.0 (15.09.2026)+, pure Go | BSD-3 | v2+ |\[20\]
| Спека | JSON (stdlib) / `yaml.v3` | — | — | JSON у v1 |

DuckDB/Parquet без cgo не перевірялися (основний Go-драйвер DuckDB, із загальних знань, використовує cgo). **Мінімальний набір v1:** stdlib + `x/time/rate` + `gonum` + `gonum/plot` + `gopdf` + `bbolt`.

**B6. Однострінковий звіт.** Згори донизу: заголовок-відповідь («Інтерес до астрономії в uk.wikipedia стабільний, не зростаючий (довіра: середня)»); 3 плитки (YoY частки, темп/рік, обсяг/міс); графік частки зі сплесками й зоною «перекласифікація ботів 2025»; таблиця мов із довірою; 2–4 причини рівня довіри; 2–3 наступні перевірки; дрібно — припущення, QID, дати, «інтерес ≠ попит». Невизначеність — інтервалами й словами («ймовірно +5–15% на рік»).

### C. Процес розробки й тестування

**C1. Claude Code (вересень 2026, за документацією).**
- **Skills:** команди злиті з навичками; `.claude/skills/` підхоплюються без перезапуску; персональні `~/.claude/skills/` не потрапляють у хмарні сесії, закомічені в репо — потрапляють; вбудовані `/verify`, `/run`, `/code-review`, `/debug`.\[2\]
- **Субагенти** (`.claude/agents/*.md`): `model` (sonnet/opus/haiku/inherit), `tools`, `disallowedTools`, `skills` (навички не успадковуються), `hooks`, `maxTurns`, `permissionMode`, `memory`, `effort`, `isolation`; невідомі поля ігноруються мовчки.\[50\]\[51\]
- **Hooks:** `PostToolUse` не блокує (exit 2 показує stderr моделі); `PreToolUse` блокує; `Stop` може змусити продовжити; типи command/http/mcp_tool/prompt/agent.\[52\]\[53\]\[54\]
- **Headless:** `claude -p "…" --output-format json|stream-json`; у `-p` тека вважається довіреною; bare mode обмежує завантаження навичок — для евалів не використовуй.\[53\]\[55\]
- **`/goal`:** після кожного ходу мала модель (за замовчуванням Haiku) перевіряє умову за транскриптом; умова має бути перевірюваною виходом команд.\[56\]\[57\]
- **Не підтверджено:** `/skill-doctor`, `claude plugin eval`, Go/gopls-плагін — перевір через `/plugin` і changelog; на захисті не стверджуй, що вони є.

**C2. Евали:** сценарії в 5.4. Метрики на запуск: pass/fail асерцій, виклики інструментів, токени, вартість, тривалість, помилки CLI. ≥3 повтори (фінал — 5); pass rate як k/n.

**C3. Стенд**

| Стенд | Роль | Обмеження |
|---|---|---|
| Claude Code + `--model haiku` headless | Основний | Вартість Haiku |
| OpenCode + OpenRouter `:free` | Переносимість + дешева модель | 20/хв, 50–1000/день, 429 upstream |
| Власний Go-харнес (`bash` + `read_file`) | Детерміновані метрики й повтори | Не відтворює вибір навички агентом |
| Ollama / llama.cpp | Опційний «найгірший випадок» | Слабкий tool calling |

**C4. OpenRouter free:** ліміти глобальні на акаунт (додаткові ключі не допомагають); від'ємний баланс дає 402 навіть на free; моделі ротуються; перевір налаштування логування/навчання для free-варіантів.\[58\]\[59\] Вторинний тест: `llama-3.3-70b:free` дав 429 у 9 з 9 спроб при невичерпаній квоті.\[60\]

**C5. Перевірка AI-коду:** TDD для `stats/` з еталонних фікстур; table-driven тести; golden JSON з `-update`; `httptest.Server` або go-vcr для записаних відповідей; `testing.F` fuzzing для спеки; `pgregory.net/rapid` для інваріантів (масштабування ряду не змінює частку-тренд; один сплеск не змінює знак нахилу); синтетичні ряди (+20%/рік + сезонність; плоский + сплеск ×50; ступінь); крос-рев'ю субагентом іншої моделі; `docs/decisions.md` і `docs/verification-log.md` як матеріал для захисту.

## 5. Чернетки артефактів

### 5.1 Дерево навички

```
.claude/skills/wiki-interest-trends/
├── SKILL.md                 # ≤200 рядків: золотий шлях + правила
├── LICENSE.txt
├── references/ METHODOLOGY.md, SPEC.md, ERRORS.md
├── assets/ fonts/NotoSans-{Regular,Bold}.ttf + OFL.txt, report-template.json
└── scripts/
    ├── go.mod               # module wikitrend; go 1.24 (без toolchain)
    ├── go.sum
    ├── cmd/wikitrend/main.go
    └── internal/ wm/ wd/ cache/ series/ stats/ trust/ chart/ pdf/ narrative/
runs/ (у .gitignore): spec.json, data.json, result.json, chart.png, report.pdf
```

Фрагмент SKILL.md:
```markdown
---
name: wiki-interest-trends
description: Analyzes Wikipedia pageview trends for a topic across language editions, estimates whether interest is growing and how trustworthy that is, and produces charts and a one-page PDF report. Use when the user asks about interest, demand, popularity or trends of a topic, course, or language market in Wikipedia, or compares languages for a B2C product launch.
compatibility: Requires Go 1.24+ on PATH (go run; downloads modules on first run) and HTTPS access to wikimedia.org and wikidata.org. Not usable in sandboxes without network.
---
1. `go -C scripts run ./cmd/wikitrend doctor` (first run may take 1–2 min).
2. `... resolve --topic "<topic>" --langs pl,cs` → show candidates, ask user to confirm.
3. `... analyze --spec runs/<id>/spec.json`
4. Write narrative using ONLY placeholders from result.json (`{{pl.yoy_pct}}`). Never type digits.
5. `... report --run <id> --narrative runs/<id>/narrative.md`
6. Follow-ups: `analyze --from <id> --set key=value`.
```

### 5.2 Контракт CLI

| Команда | Аргументи | Дія |
|---|---|---|
| `doctor [--warmup]` | — | Версія Go, мережа, кеш; прогрів збірки |
| `resolve` | `--topic`, `--langs`, `--basket` | Кандидати Wikidata + sitelinks + редиректи → чернетка `spec.json` |
| `analyze` | `--spec` або `--from <id> --set k=v` | Дані (кеш), частка, статистика, довіра → `result.json` |
| `report` | `--run`, `--narrative` | Графіки + PDF; валідатор цифр; перевірка 1 сторінки |
| `explain` | `--run`, `--lang` | Детальні причини довіри без сирих рядів |

```json
{"status":"ok","run_id":"r_7f3a2c","parent_run_id":null,
 "period":{"start":"2024-09","end":"2026-08","months":24},
 "results":[
  {"lang":"pl","monthly_views_median":18400,"share_yoy_pct":12.4,
   "share_trend_pct_per_year":{"est":10.9,"ci90":[3.1,18.2]},"trust":"medium",
   "reasons":["growth consistent across windows","1 spike removed (2025-01)","series crosses 2025 bot reclassification"]},
  {"lang":"cs","monthly_views_median":2100,"share_yoy_pct":-3.0,
   "share_trend_pct_per_year":{"est":-1.5,"ci90":[-9.8,6.9]},"trust":"low",
   "reasons":["no significant trend (MK p=0.41)","low volume"]}],
 "files":{"result":"runs/r_7f3a2c/result.json","chart":"runs/r_7f3a2c/chart.png"},
 "next_step":"Write narrative with placeholders, then run: report --run r_7f3a2c --narrative runs/r_7f3a2c/narrative.md"}
```
Помилка: `{"status":"error","code":"RATE_LIMITED","retry_after_s":30,"hint":"Wikimedia returned 429; cached data kept","next_step":"wait 30s, rerun analyze --spec runs/r_7f3a2c/spec.json"}`.

### 5.3 Рівень довіри (пороги — гіпотези)

| # | Перевірка | Прохід |
|---|---|---|
| C1 Напрям | (Сезонний) MK, p < 0.05, знак Theil–Sen збігається | так/ні |
| C2 Величина | YoY частки ≥ +10% і 90% інтервал нахилу > 0 | так/ні |
| C3 Сплески | Після Hampel знак той самий, величина змінилася < 50%; топ-3 дні < 20% річної суми | так/ні |
| C4 Обсяг | Медіана ≥ 1000 переглядів/міс (< 300 — автоматично «низький») | так/ні |
| C5 Чутливість | Знак стабільний у вікнах 24/36 міс і при зсуві ±3 міс | так/ні |
| C6 Злам | Зростання не зосереджене на березні–травні 2025 | так/ні |
| C7 Кошик | ≥ 2/3 статей мають той самий знак | так/ні/н.з. |

**Високий:** C1–C5 + (C6 або C7). **Середній:** C1, але провалено одне з C2/C3/C5/C6. **Низький:** провалено C1 або C4, або зростання зникає без сплесків. Калібрування: 500 синтетичних рядів на клас (+5/+15/+30% на рік; нуль + сплески; ступінь); цілі — «високий» на нульових ≤ 5%, на +15% ≥ 70%.

Формулювання: високий — «Інтерес стабільно зростає: ≈{{trend}} на рік; висновок не залежить від періоду й сплесків»; середній — «Ознаки зростання є, але частково пояснюються сплеском у {{spike_month}} / змінами обліку ботів 2025; розглядайте як гіпотезу»; низький — «Даних замало або тренд не відрізняється від шуму; рішень на цій основі не приймайте».

### 5.4 Евал-сценарії

| # | Сценарій | Критерії успіху |
|---|---|---|
| 1 | Інтервальне голодування pl vs cs, 2 роки | Обидві мови; months=24; PDF 1 сторінка |
| 2 | Астрономія uk + довіра | Рівень + ≥2 причини; немає цифр поза плейсхолдерами |
| 3 | Вивчення англійської, 4 мови, «кого далі» | Кошик підтверджено; ранжування; «мова ≠ країна» |
| 4 | «А тепер за 3 роки і без мобільних» | `analyze --from`, `parent_run_id`, без повторного resolve |
| 5 | Неоднозначна тема («Меркурій») | Показує кандидатів, не вгадує |
| 6 | Відсутня стаття в одній мові | `missing` у JSON, пояснення, без падіння |
| 7 | Малий обсяг | «низький», причина low volume |
| 8 | Фальшиве зростання від сплеску | Тренд зникає після Hampel; не «високий» |
| 9 | Ряд через 2025 рік | Попередження про перекласифікацію |
| 10 | 429 (фікстура) | Виконує `next_step`, не вигадує дані |
| 11 | «Просто скажи число» | Число з result.json + довіра |
| 12 | Запит не про Wikipedia | Навичка не активується |
| 13 | Власний критерій перспективності | У spec, застосовано кодом |
| 14 | Повторний ідентичний запит | Кеш, ті самі числа, менше викликів |
| 15 | Go відсутній | `doctor` → зрозуміла помилка, агент не пише код сам |

Оцінювання: програмні асерції по `result.json`, кількості сторінок PDF і числах; LLM-суддя (Sonnet/Opus) за рубрикою 1–5; ручна вибірка 20% запусків.

### 5.5 Структура .claude/ і етапи

```
CLAUDE.md          # стек, make-команди, правила: pure Go, CGO_ENABLED=0, без бінарників, TDD для stats/, JSON ≤2KB
.claude/
├── settings.json  # hooks + permissions (Bash(go *), Bash(make *))
├── hooks/go-format.sh  # PostToolUse Edit|Write *.go → gofmt -w + goimports -w
├── hooks/go-check.sh   # Stop → go vet ./... && go test ./... -short (exit 2 → продовжити)
├── agents/go-reviewer.md     # model: opus, Read/Grep/Glob
├── agents/stats-verifier.md  # model: sonnet, звіряє stats/ з еталонами
├── agents/skill-tester.md    # model: haiku, skills: [wiki-interest-trends]
├── agents/eval-judge.md      # model: sonnet, рубрика, лише читання runs/
└── skills/wiki-interest-trends/
Makefile: test, lint (golangci-lint), eval-smoke, eval-full, fixtures, vendor
evals/: scenarios.json, rubric.md, run.sh (claude -p … --model haiku --output-format json)
docs/: decisions.md, verification-log.md
```

Етапи: (1) plan mode й ADR; (2) клієнт Wikimedia + фікстури; (3) `stats/` через TDD; (4) `resolve`/`analyze` + golden JSON; (5) графік, PDF, валідатор; (6) SKILL.md + `skills-ref validate`; (7) евали на Haiku ×3 та ітерації; (8) OpenCode + free-модель; (9) `/goal "make eval-full shows ≥13/15 on haiku for 3 runs"`; (10) документи для захисту.

### 5.6 Roadmap

- **v1 (здача):** 1–3 мови, стаття або малий кошик, місячна частка, 3 рівні довіри, PDF, файловий кеш/bbolt, 15 евалів.
- **v2:** багато тем × мов в одній спеці; зважений скор перспективності; `modernc.org/sqlite`; горутини через один спільний `rate.Limiter` (≤200/хв із запасом); інкрементальне оновлення нових місяців.
- **v3:** дампи dumps.wikimedia.org для сотень і тисяч статей; колонкове сховище (Parquet, перевір pure-Go); семантичне групування тем через ембедінги (Qdrant доречний саме тут); `top-by-country` і unique devices.
- **v4:** моніторинг за розкладом (routines/cron); окремий Go-сервіс лише для кількох користувачів: PostgreSQL — історія й спеки, Redis — кеш і лімітер, NATS — черга збору. Для однокористувацької навички це зайве й порушує вимогу «самостійна навичка».

## 6. Топ-10 пасток

Загальні:
1. Абсолюти замість частки — падіння Wikipedia на 8–10% р/р ховає справжнє зростання.
2. `all-agents` замість `user`.
3. Ігнорування редиректів.
4. Порівняння двох кінцевих точок — один сплеск визначає «тренд».
5. 404 як помилка (а не нуль) або неповний поточний місяць.
6. Сирі ряди в контексті моделі.
7. Модель сама пише числа.
8. p-value як «довіра» (автокореляція, множинні порівняння).
9. Мова = країна (en/es/pt/fr).
10. Евали по одному прогону.

Go-специфічні:
1. Дефолтний `Go-http-client` UA → блок або тир 10/хв.
2. `toolchain` чи надто новий `go` + `GOTOOLCHAIN=local` → не збирається.
3. Випадкова cgo-залежність (тестуй з `CGO_ENABLED=0`).
4. Довгий перший `go run` перериває агент — `doctor --warmup` і попередження.
5. Відносні шляхи від cwd; `os.Executable` у `go run` вказує в temp — бери `go -C` і `embed`.
6. Необмежені горутини без спільного лімітера.
7. Архівні бібліотеки (fpdf, go-chart) зі старих прикладів AI.
8. Неокруглені `float` у golden-файлах.
9. Невбудований шрифт → немає кирилиці/діакритики на Windows.
10. Симлінки на Windows і CRLF у `.sh`-хуках (налаштуй `.gitattributes`).

## 7. Відкриті питання

1. Мінімальна версія Go в `go.mod` — перевір `go version` у хмарній сесії Claude Code.
2. JSON чи YAML для спеки (рекомендую JSON у v1).
3. Пороги довіри — після калібрування.
4. Чи включати `mobile-app`.
5. Межа «кошик vs одна стаття» і хто затверджує кошик.
6. Мова PDF.
7. Бюджет евалів: Haiku і $10 кредитів OpenRouter.
8. Офіційна підтримка Codex/Gemini CLI чи «best effort».

## Застереження

- Документація Claude Code й OpenRouter змінюється щотижня — перевір `claude --version` і changelog перед захистом.
- Цифра падіння трафіку −8% узята з першоджерела — поста Marshall Miller «New User Trends on Wikipedia» у блозі WMF Diff (17.10.2025), де сама Wikimedia застерігає щодо порівнянь через зміну детекції ботів; −10.6% — неофіційний репозиторій.
- Ліміти OpenRouter free підтверджено довідковим центром OpenRouter («OpenRouter Rate Limits»), але не сторінкою документації `:free`, тож перевір їх у своєму акаунті; ліміти Wikimedia 2026 офіційно експериментальні.
- Параметри Wikidata API й назви еталонних Python/R-функцій — із загальних знань; час першого `go run` — гіпотеза.

## Sources

1. <https://agentskills.io/specification>
2. [Extend Claude with skills - Claude Code Docs](https://code.claude.com/docs/en/skills)
3. [Configure cloud environments - Claude Code Docs](https://code.claude.com/docs/en/cloud-environments)
4. [Code execution tool - Claude Platform Docs](https://platform.claude.com/docs/en/agents-and-tools/tool-use/code-execution-tool)
5. [Agent Skills - Claude Platform Docs](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview)
6. [T400119 Block traffic from user-agents not honoring our policy](https://phabricator.wikimedia.org/T400119)
7. [Getting started](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/documentation/getting-started.html)
8. [Page view analytics](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/reference/page-views.html)
9. [Changelog](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/changelog.html)
10. [Page views](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/concepts/page-views.html)
11. [Pageviews Analysis - FAQ](https://pageviews.toolforge.org/faq)
12. <https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/documentation/troubleshooting.html>
13. [When Open-Source Go PDF Libraries Fail: A Migration Guide](https://unidoc.io/post/go-pdf-library-alternative-migration/)
14. [GitHub - jung-kurt/gofpdf: A PDF document generator with high level support for text, drawing and images · GitHub](https://github.com/jung-kurt/gofpdf)
15. [gopdf package - github.com/signintech/gopdf - Go Packages](https://pkg.go.dev/github.com/signintech/gopdf)
16. [gpdf — Pure Go PDF Generation Library (MIT, 0 deps)](https://gpdf.dev/blog/go-pdf-library-showdown-2026/)
17. [GitHub - wcharczuk/go-chart: go chart is a basic charting library in go. · GitHub](https://github.com/wcharczuk/go-chart)
18. [plot package - gonum.org/v1/plot - Go Packages](https://pkg.go.dev/gonum.org/v1/plot)
19. [bbolt package - go.etcd.io/bbolt - Go Packages](https://pkg.go.dev/go.etcd.io/bbolt)
20. [sqlite package - modernc.org/sqlite - Go Packages](https://pkg.go.dev/modernc.org/sqlite)
21. [stat package - gonum.org/v1/gonum/stat - Go Packages](https://pkg.go.dev/gonum.org/v1/gonum/stat)
22. [Claude Code Integration - OpenRouter](https://openrouter.ai/docs/cookbook/coding-agents/claude-code-integration)
23. [Claude Code with OpenRouter: Setup, Models, and Costs](https://openrouter.ai/blog/tutorials/claude-code-openrouter/)
24. [10 OpenCode Skills Worth Installing in 2026](https://www.firecrawl.dev/blog/best-opencode-skills)
25. [Improve toolchain handling by matthewhughes934 · Pull Request #460 · actions/setup-go](https://github.com/actions/setup-go/pull/460)
26. [Understand Go toolchain directive or your money back :: Alex Bozhenko](https://alexbozhenko.github.io/posts/2024-12-19-understand-go-toolchain-directive-or-your-money-back/)
27. [Access policy](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/documentation/access-policy.html)
28. [Page metrics](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/examples/page-metrics.html)
29. [Wikimedia Pageviews & Analytics Scraper · Apify](https://apify.com/scrapyx/wikimedia-pageviews-scraper)
30. [Project metrics](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/examples/project-metrics.html)
31. [Country data](https://doc.wikimedia.org/generated-data-platform/aqs/analytics-api/concepts/country-data.html)
32. [Data Platform/Systems/AQS - Wikitech](https://wikitech.wikimedia.org/wiki/Analytics/Systems/AQS)
33. [GitHub - Sagorika28/English-Wikipedia-Traffic-Analysis: This project analyzes monthly traffic patterns on English Wikipedia from December 2007 through August 2025. By combining data from two different Wikimedia API endpoints, we create a comprehensive dataset that tracks the evolution of Wikipedia usage across desktop and mobile platforms over nearly two decades. · GitHub](https://github.com/Sagorika28/English-Wikipedia-Traffic-Analysis)
34. [Pageviews Analysis - Meta-Wiki - Wikimedia](https://meta.wikimedia.org/wiki/Pageviews_Analysis)
35. [Requests to Wikidata get rate limited because of insufficient User-Agent · Issue #7731 · OpenRefine/OpenRefine](https://github.com/OpenRefine/OpenRefine/issues/7731)
36. [Introducing the unique devices dataset: a new way to estimate reach on Wikimedia projects](https://diff.wikimedia.org/2016/03/30/unique-devices-dataset/)
37. [What Wikipedia Reveals About AI Overviews And Web Traffic](https://www.searchenginejournal.com/what-wikipedia-reveals-about-ai-overviews-and-web-traffic/589042/)
38. [AI-research-SKILLs/anthropic\_official\_docs/best\_practices.md at main · firecrawl/AI-research-SKILLs](https://github.com/firecrawl/AI-research-SKILLs/blob/main/anthropic_official_docs/best_practices.md)
39. [Skill authoring best practices - Claude Platform Docs](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
40. [Agent Skills Support: Which AI Clients Read SKILL.md](https://www.skillsboard.sh/agent-skills-support)
41. [GitHub - vercel-labs/skills: The open agent skills tool - npx skills · GitHub](https://github.com/vercel-labs/skills)
42. [Agent skills - TWG CLI](https://developer.atlassian.com/cloud/twg-cli/agents/skills/)
43. [Skill authoring best practices - Claude Docs](https://anthropic.mintlify.app/en/docs/agents-and-tools/agent-skills/best-practices)
44. [Skill Authoring Patterns from Anthropic’s Best Practices](https://generativeprogrammer.com/p/skill-authoring-patterns-from-anthropics)
45. [apply-anthropic-skill-best-practices by neolabhq — @skills](https://atskills.one/neolabhq/apply-anthropic-skill-best-practices)
46. [skill-creator](https://www.x-cmd.com/skill/anthropics/skill-creator/)
47. [Claude Code Skills 2.0: Evals, Benchmarks and A/B Testing for Skills that Actually Work](https://pasqualepillitteri.it/en/news/341/claude-code-skills-2-0-evals-benchmarks-guide)
48. [Skills Without Evals Are Just Markdown and Hope - DEV Community](https://dev.to/danielsogl/skills-without-evals-are-just-markdown-and-hope-3a71)
49. [\_content/doc/toolchain.md](https://go.googlesource.com/website/+/refs/heads/master/_content/doc/toolchain.md)
50. [Create custom subagents - Claude Code Docs](https://code.claude.com/docs/en/sub-agents)
51. [Claude Code Subagents: A 2026 Practical Guide - Tembo.io](https://www.tembo.io/blog/claude-code-subagents)
52. [Hooks reference - Claude Code Docs](https://code.claude.com/docs/en/hooks)
53. [Claude Code Hooks: PreToolUse, PostToolUse, and Practical Examples](https://harnessrouter.ai/blog/claude-code-hooks)
54. [Claude Code Hooks in 2026: A Production Playbook (PreToolUse, PostToolUse, Stop, SubagentStop) - Totalum Blog](https://www.totalum.app/blog/claude-code-hooks-totalum)
55. [How to Use Claude Code Hooks for Automation](https://inventivehq.com/knowledge-base/claude/how-to-use-hooks-for-automation)
56. [Claude Code's /goal Command - by Avi Chawla](https://blog.dailydoseofds.com/p/claude-codes-goal-command)
57. [Keep Claude working toward a goal - Claude Code Docs](https://code.claude.com/docs/en/goal)
58. [OpenRouter Free Credits: What the Limit Really Means (2026)](https://aireiter.com/blog/openrouter-free-credits-limit)
59. [OpenRouter Free Models: Limits and How to Raise Them](https://benchlm.ai/free-tier/openrouter)
60. [OpenRouter Free Models Tested: Daily Limits, 429s, and When to Use Them · Productize](https://productize.life/blog/openrouter-free-models/en)
