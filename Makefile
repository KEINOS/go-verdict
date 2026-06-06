.PHONY: help all check test test-verbose test-e2e lint
.PHONY: build build-binary verify-build
.PHONY: clean-dist clean-testdata clean
.PHONY: data data-cache-hit data-cache-key data-cache-save data-generate
.PHONY: data-example-mismatch data-benchstat-repeat-fast data-gotestbench data-insufficient
.PHONY: e2e
.PHONY: check-checkmake-exist check-golangci-lint-exist check-markdownlint-cli2-exist check-yamlfmt-exist
.PHONY: lint-e2e lint-go lint-makefile lint-md lint-yaml

VERDICT := ./dist/verdict
SHASUM := shasum -a 1
DATA_CACHE := ./testdata/e2e-fixtures/cache.txt
DATA_CACHE_KEY := ./testdata/e2e-fixtures/cache_key.txt
DATA_CACHE_INPUTS := ./go.mod ./go.sum ./testdata/benchtargets/targets.go
DATA_FIXTURES := \
	./testdata/e2e-fixtures/bench_ExampleFast.txt \
	./testdata/e2e-fixtures/bench_ExampleSlow.txt \
	./testdata/e2e-fixtures/bench_old.txt \
	./testdata/e2e-fixtures/bench_new.txt \
	./testdata/e2e-fixtures/bench_gotestbench.txt \
	./testdata/e2e-fixtures/bench_gotestbench_count2.txt \
	./testdata/e2e-fixtures/benchstat_Example.txt \
	./testdata/e2e-fixtures/benchstat_E2E.txt \
	./testdata/e2e-fixtures/benchstat_csv_E2E.txt
HELP_LINES := \
	'Development targets (run from the repository root):' \
	'  make all                  Run make check build.' \
	'  make check                Run all validation gates: test, lint, and e2e.' \
	'  make test                 Run unit tests with race detector and coverage.' \
	'  make test-e2e             Build verdict, prepare fixtures, then run YAML E2E scenarios.' \
	'  make lint                 Run mutating fixers: go fix, golangci-lint --fix, markdownlint-cli2 --fix, yamlfmt.' \
	'  make build                Remove ./dist, then build ./dist/verdict.' \
	'  make clean-dist           Remove ./dist.' \
	'  make clean-testdata       Remove generated testdata/e2e-fixtures/*.txt files.' \
	'  make clean                Remove generated testdata/e2e-fixtures/*.txt files and ./dist.' \
	'' \
	'Fixture targets:' \
	'  make data                 Ensure benchmark fixtures exist and are current.' \
	'  make data-gotestbench     Regenerate the raw go test -bench fixture.' \
	'' \
	'End-to-end targets:' \
	'  make e2e                  Alias for make test-e2e.'

help:
	@printf '%s\n' $(HELP_LINES)

all: check build

build: clean-dist build-binary verify-build

build-binary:
	@printf '\n== Build: compiling ./cmd/verdict to ./dist/verdict ==\n'
	@mkdir -p ./dist
	@go build -ldflags="-s -w" -trimpath -o $(VERDICT) ./cmd/verdict

check: test lint e2e

check-checkmake-exist:
	@command -v checkmake >/dev/null 2>&1 || (echo "FAIL: checkmake command is not installed" && exit 1)

check-golangci-lint-exist:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "FAIL: golangci-lint command is not installed" && exit 1)

check-markdownlint-cli2-exist:
	@command -v markdownlint-cli2 >/dev/null 2>&1 || (echo "FAIL: markdownlint-cli2 command is not installed" && exit 1)

check-yamlfmt-exist:
	@command -v yamlfmt >/dev/null 2>&1 || (echo "FAIL: yamlfmt command is not installed" && exit 1)

clean: clean-testdata clean-dist

clean-dist:
	@printf '\n== Clean: removing ./dist ==\n'
	@rm -rf ./dist && printf 'Removed ./dist.\n'

clean-testdata:
	@printf '\n== Clean: removing generated testdata/e2e-fixtures/*.txt files ==\n'
	@rm -f ./testdata/e2e-fixtures/*.txt

data: data-cache-key
	@if $(MAKE) --no-print-directory --silent data-cache-hit > /dev/null 2>&1; then printf '\n== Data: using cached benchmark fixtures ==\n'; else $(MAKE) --no-print-directory data-generate; fi

data-benchstat-repeat-fast:
	@printf '\n== Generate benchstat E2E fixture: repeat BenchmarkExampleFast as old/new, with no intentional implementation difference ==\n'
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata/benchtargets > ./testdata/e2e-fixtures/bench_old.txt
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata/benchtargets > ./testdata/e2e-fixtures/bench_new.txt
	benchstat ./testdata/e2e-fixtures/bench_old.txt ./testdata/e2e-fixtures/bench_new.txt > ./testdata/e2e-fixtures/benchstat_E2E.txt
	benchstat -format csv ./testdata/e2e-fixtures/bench_old.txt ./testdata/e2e-fixtures/bench_new.txt > ./testdata/e2e-fixtures/benchstat_csv_E2E.txt

data-cache-hit:
	@test -f $(DATA_CACHE)
	@$(SHASUM) --check --status $(DATA_CACHE)
	@for fixture in $(DATA_FIXTURES); do test -f "$$fixture" || exit 1; done

data-cache-key:
	@mkdir -p ./testdata/e2e-fixtures
	@$(SHASUM) $(DATA_CACHE_INPUTS) > $(DATA_CACHE_KEY)

data-cache-save:
	@$(MAKE) --no-print-directory data-cache-key
	@$(SHASUM) $(DATA_CACHE_KEY) > $(DATA_CACHE)

data-example-mismatch:
	@printf '\n== Generate example fixture: different benchmark names, useful for mismatch examples ==\n'
	go test -bench=BenchmarkExampleFast -count=10 ./testdata/benchtargets | tee ./testdata/e2e-fixtures/bench_ExampleFast.txt
	go test -bench=BenchmarkExampleSlow -count=10 ./testdata/benchtargets | tee ./testdata/e2e-fixtures/bench_ExampleSlow.txt
	benchstat ./testdata/e2e-fixtures/bench_ExampleFast.txt ./testdata/e2e-fixtures/bench_ExampleSlow.txt > ./testdata/e2e-fixtures/benchstat_Example.txt

data-generate:
	@$(MAKE) --no-print-directory data-example-mismatch
	@$(MAKE) --no-print-directory data-benchstat-repeat-fast
	@$(MAKE) --no-print-directory data-gotestbench
	@$(MAKE) --no-print-directory data-insufficient
	@$(MAKE) --no-print-directory data-cache-save

data-gotestbench:
	@printf '\n== Generate gotestbench E2E fixture: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata/benchtargets > ./testdata/e2e-fixtures/bench_gotestbench.txt

data-insufficient:
	@printf '\n== Generate insufficient raw samples E2E fixture: count=2 asks for more samples ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=2 ./testdata/benchtargets > ./testdata/e2e-fixtures/bench_gotestbench_count2.txt

e2e: test-e2e

lint: lint-e2e lint-go lint-makefile lint-md lint-yaml

lint-e2e: check-golangci-lint-exist
	@echo "* Modernizing end-to-end tests go syntax..."
	@go fix -tags=e2e ./tools/e2e/... && echo "0 issues."
	@echo "* Running end-to-end tests lint..."
	@golangci-lint run --fix --build-tags "e2e" --timeout 5m ./tools/e2e/...

lint-go: check-golangci-lint-exist
	@echo "* Modernizing go syntax..."
	@go fix ./... && echo "0 issues."
	@echo "* Running go lint w/fix..."
	@golangci-lint run --fix --timeout 5m

lint-makefile: check-checkmake-exist
	@echo "* Running Makefile lint..."
	@checkmake Makefile && echo "0 issues."

lint-md: check-markdownlint-cli2-exist
	@echo "* Running markdown lint w/fix..."
	@markdownlint-cli2 --fix "**/*.md" 1>/dev/null && echo "0 issues."

lint-yaml: check-yamlfmt-exist
	@echo "* Running YAML formatter w/fix..."
	@yamlfmt .golangci.yml .markdownlint-cli2.yaml .yamlfmt ./testdata/e2e-scenarios && echo "0 issues."

test:
	go test -cover -race ./...

test-e2e: build data
	@echo "* Running end-to-end tests..."
	@VERDICT_BIN="$(CURDIR)/dist/verdict" \
	 VERDICT_E2E_SCENARIOS_DIR="$(CURDIR)/testdata/e2e-scenarios" \
	 go test -tags=e2e -race ./tools/e2e/...

test-verbose:
	go test -v -cover -race ./...

verify-build:
	@$(VERDICT) -v 1> /dev/null
	@$(VERDICT) --version 1> /dev/null
	@$(VERDICT) version 1> /dev/null
	@$(VERDICT) -h 1> /dev/null
	@printf 'Built ./dist/verdict successfully.\n'
