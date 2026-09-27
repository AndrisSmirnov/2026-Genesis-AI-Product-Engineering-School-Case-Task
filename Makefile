SKILL_DIR := .claude/skills/wiki-interest-trends
SCRIPTS   := $(SKILL_DIR)/scripts
export CGO_ENABLED := 0

.PHONY: test test-short lint eval-smoke eval-full fixtures demo

test:          ## all Go tests incl. simulations/calibration
	cd $(SCRIPTS) && go test ./...

test-short:    ## fast tests (no simulations)
	cd $(SCRIPTS) && go test -short ./...

lint:
	cd $(SCRIPTS) && gofmt -l . && go vet ./...

eval-smoke:    ## 3 key scenarios on Claude Haiku 4.5 (needs `claude` CLI)
	$(SKILL_DIR)/evals/run.sh astro_uk followup_cache english_basket_report

eval-full:     ## all scenarios × 3 runs on Haiku
	REPEAT=3 $(SKILL_DIR)/evals/run.sh

fixtures:      ## regenerate scipy/pymannkendall reference values
	python3 -m venv $(SKILL_DIR)/.venv && $(SKILL_DIR)/.venv/bin/pip install -q scipy pymannkendall numpy
	$(SKILL_DIR)/.venv/bin/python $(SKILL_DIR)/dev/gen_fixtures.py

demo:          ## end-to-end run on live data: astronomy in uk + pl
	$(SCRIPTS)/wikitrend doctor
	$(SCRIPTS)/wikitrend resolve --topic "astronomy" --langs uk,pl
