# Genesis — навичка wiki-interest-trends

Agent Skill для аналізу переглядів Wikipedia: чи зростає інтерес до теми в мовних розділах і наскільки цьому довіряти. Результат — графік і PDF на одну сторінку. Навичка має працювати на дешевій моделі (Haiku 4.5, OpenRouter `:free`).

## Документи
- Завдання: `docs/research/task.md`
- Дослідження (основа архітектури): `docs/research/wikipedia-trends-go.md`. Читай за потреби, не імпортуй цілком.
- Рішення: `DECISIONS.md`. Джерело істини для архітектури.
- Журнал перевірок AI: `VERIFICATION.md`

## Стек
- Go, pure Go, `CGO_ENABLED=0`. Жодних скомпільованих бінарників у репо.
- Навичка: `.claude/skills/wiki-interest-trends/`. Код лежить у `scripts/`, запускається через обгортку `scripts/wikitrend <cmd>` (усередині — `go run ./cmd/wikitrend`).
- У `go.mod` лише рядок `go 1.22` (перевірено: 1.21 не збирає), без `toolchain`. Єдина залежність — `signintech/gopdf`.
- Евали: `evals/run.sh` запускає headless Claude Code на Haiku в тимчасових папках **поза git-репозиторієм** (інакше Claude Code підхоплює оригінал навички).

## Команди
- `make test` — усі тести, включно з калібруванням; `make test-short` — швидкі
- `make lint` — `gofmt -l` + `go vet`
- `make eval-smoke` / `make eval-full` — евали на Haiku
- `make fixtures` — перегенерувати еталони `scipy` для `stats`

## Правила
1. **Архітектурні рішення не ухвалюєш сам.** Якщо рішення немає в `DECISIONS.md`, зупинись і спитай. Не заповнюй `DECISIONS.md` за користувача.
2. **Гіпотези ≠ факти.** Пороги довіри, параметри Wikidata API, версія Go, час першого `go run` у дослідженні позначені як гіпотези. Перевір їх реальним запуском, перш ніж на них спиратися. Результат перевірки запиши в `VERIFICATION.md`.
3. **TDD для `internal/stats/`.** Спершу тест з еталонними значеннями, потім код.
4. **Контракт CLI для агента:** stdout ≤2 КБ JSON (`status`, `summary`, `warnings`, `next_step`, шляхи файлів). Сирі ряди пишуться лише у файли `runs/<id>/`. Помилки віддаються з `code`, `hint`, `next_step` і ненульовим exit code.
5. **Контракт CLI не змінюється без оновлення евалів** (`evals/scenarios.json`).
6. **Кожен HTTP-запит до Wikimedia/Wikidata має явний User-Agent** формату `wikitrend/<ver> (<contact>) go/<ver>`. Дефолтний `Go-http-client` блокують.
7. **Модель не пише числа.** Текст звіту містить лише плейсхолдери `{{...}}`, числа підставляє код.
8. Метрика: частка теми від агрегату розділу з `agent=user`. Редиректи сумуються. 404 означає нуль.
9. Без архівних бібліотек: `go-pdf/fpdf`, `jung-kurt/gofpdf`, `wcharczuk/go-chart`.

## Субагенти
`go-reviewer`, `stats-verifier`, `skill-tester` (Haiku), `eval-judge` лежать у `.claude/agents/`.
