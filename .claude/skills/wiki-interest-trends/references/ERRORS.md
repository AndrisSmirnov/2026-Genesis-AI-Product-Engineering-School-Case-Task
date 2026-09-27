# Error codes

Every error is printed as JSON on stdout: `{"status":"error","code":…,"hint":…,"next_step":…}`. The exit code is non-zero. Always follow `next_step`.

| Code | Meaning | What to do |
|---|---|---|
| `GO_MISSING` | Go is not installed | Ask the user to install Go 1.22+ (https://go.dev/dl/), then run `doctor` again. Never write the analysis yourself. |
| `NETWORK` | wikimedia.org / wikidata.org cannot be reached | Tell the user the skill needs internet access. Do not invent data. |
| `RATE_LIMITED` | Wikimedia returned 429 | Wait `retry_after_s` seconds and rerun the same command. Data already downloaded is cached. |
| `UPSTREAM` | Wikimedia 5xx or an unexpected answer | Wait a minute and rerun. |
| `TOPIC_NOT_FOUND` | No Wikidata entity with articles in these languages | Ask the user for another wording (English names work best) or other languages. |
| `NO_ARTICLES` | The entity exists, but there is no article in any requested language | Tell the user; suggest a broader topic or other languages. |
| `NO_DATA` | No aggregate pageviews for a language code | Check the language code. |
| `BAD_ARGS` | Wrong flag or value | Read `hint`, fix the command. |
| `NEEDS_RESOLVE` | `--set langs=` added a language that was never resolved | Run the `resolve` command from `next_step`. |
| `SPEC_NOT_FOUND` / `RUN_NOT_FOUND` | Wrong path or run_id | Use the values printed by the previous command. |
| `INSUFFICIENT_DATA` | Not enough months for the window | Use a shorter window or a later end month. |
| `RAW_NUMBER` | The narrative contains digits | Replace each number (see `details.snippets`) with a placeholder, or rephrase without numbers. |
| `UNKNOWN_PLACEHOLDER` | A placeholder that does not exist | Use only names from `details.available`. |
| `BAD_NARRATIVE` | No headline or no body | The first line is the headline; paragraphs follow. |
| `NARRATIVE_TOO_LONG` | The report would not fit on one page | Shorten the text by the amount given in `hint`. |

Statuses that are not errors:
- `needs_choice` — the topic is ambiguous. Show the candidates and ask the user.
