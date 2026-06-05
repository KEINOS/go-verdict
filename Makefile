.PHONY: help check test test-verbose lint
.PHONY: build build-binary verify-build
.PHONY: clean-dist clean-testdata clean-all
.PHONY: data data-cache-hit data-cache-key data-cache-save data-generate
.PHONY: data-example-mismatch data-benchstat-repeat-fast data-gotestbench data-insufficient
.PHONY: e2e e2e-readme-pipelines
.PHONY: e2e-benchstat e2e-benchstat-text e2e-benchstat-text-output e2e-benchstat-text-assert
.PHONY: e2e-benchstat-csv e2e-benchstat-csv-output e2e-benchstat-csv-assert
.PHONY: e2e-gotestbench e2e-gotestbench-output e2e-gotestbench-assert
.PHONY: e2e-ab e2e-ab-mismatch e2e-ab-explicit
.PHONY: e2e-insufficient e2e-insufficient-assert

VERDICT := ./dist/verdict
SHASUM := shasum -a 1
DATA_CACHE := ./testdata/cache.txt
DATA_CACHE_KEY := ./testdata/cache_key.txt
DATA_CACHE_INPUTS := ./go.mod ./go.sum ./testdata/benchtargets/targets.go
DATA_FIXTURES := ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt ./testdata/bench_old.txt ./testdata/bench_new.txt ./testdata/bench_gotestbench.txt ./testdata/bench_gotestbench_count2.txt ./testdata/benchstat_Example.txt ./testdata/benchstat_E2E.txt ./testdata/benchstat_csv_E2E.txt

help:
	@printf '%s\n' 'Development targets (run from the repository root):' '  make check                Run all validation gates: test, lint, and e2e.' '  make test                 Run unit tests with race detector and coverage.' '  make lint                 Run mutating fixers: go fix, golangci-lint --fix, markdownlint-cli2 --fix.' '  make build                Remove ./dist, then build ./dist/verdict.' '  make clean-dist           Remove ./dist.' '  make clean-testdata       Remove generated testdata/*.txt files.' '  make clean-all            Remove generated testdata/*.txt files and ./dist.' '' 'Fixture targets:' '  make data                 Ensure benchmark fixtures exist and are current.' '  make data-gotestbench     Regenerate the raw go test -bench fixture.' '' 'End-to-end targets (build first and assume repo-root paths):' '  make e2e                  Run all E2E checks.' '  make e2e-benchstat        Check benchstat stdin parsing.' '  make e2e-gotestbench      Check raw go test -bench stdin parsing.' '  make e2e-ab               Check explicit raw-file A/B comparison.' '  make e2e-insufficient     Check insufficient raw sample guidance.'

check: test lint e2e

test:
	go test -cover -race ./...

test-verbose:
	go test -v -cover -race ./...

lint:
	go fix ./...
	golangci-lint run --fix --timeout 5m
	markdownlint-cli2 --fix "**/*.md"

build: clean-dist build-binary verify-build

build-binary:
	@printf '\n== Build: compiling ./cmd/verdict to ./dist/verdict ==\n'
	@mkdir -p ./dist
	@go build -ldflags="-s -w" -trimpath -o $(VERDICT) ./cmd/verdict

verify-build:
	@$(VERDICT) -v 1> /dev/null
	@$(VERDICT) --version 1> /dev/null
	@$(VERDICT) version 1> /dev/null
	@$(VERDICT) -h 1> /dev/null
	@printf 'Built ./dist/verdict successfully.\n'

data: data-cache-key
	@if $(MAKE) --no-print-directory --silent data-cache-hit > /dev/null 2>&1; then printf '\n== Data: using cached benchmark fixtures ==\n'; else $(MAKE) --no-print-directory data-generate; fi

data-cache-key:
	@$(SHASUM) $(DATA_CACHE_INPUTS) > $(DATA_CACHE_KEY)

data-cache-hit:
	@test -f $(DATA_CACHE)
	@$(SHASUM) --check --status $(DATA_CACHE)
	@for fixture in $(DATA_FIXTURES); do test -f "$$fixture" || exit 1; done

data-cache-save:
	@$(MAKE) --no-print-directory data-cache-key
	@$(SHASUM) $(DATA_CACHE_KEY) > $(DATA_CACHE)

data-generate:
	@$(MAKE) --no-print-directory data-example-mismatch
	@$(MAKE) --no-print-directory data-benchstat-repeat-fast
	@$(MAKE) --no-print-directory data-gotestbench
	@$(MAKE) --no-print-directory data-insufficient
	@$(MAKE) --no-print-directory data-cache-save

data-example-mismatch:
	@printf '\n== Generate example fixture: different benchmark names, useful for mismatch examples ==\n'
	go test -bench=BenchmarkExampleFast -count=10 ./testdata/benchtargets | tee ./testdata/bench_ExampleFast.txt
	go test -bench=BenchmarkExampleSlow -count=10 ./testdata/benchtargets | tee ./testdata/bench_ExampleSlow.txt
	benchstat ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt > ./testdata/benchstat_Example.txt

data-benchstat-repeat-fast:
	@printf '\n== Generate benchstat E2E fixture: repeat BenchmarkExampleFast as old/new, with no intentional implementation difference ==\n'
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata/benchtargets > ./testdata/bench_old.txt
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata/benchtargets > ./testdata/bench_new.txt
	benchstat ./testdata/bench_old.txt ./testdata/bench_new.txt > ./testdata/benchstat_E2E.txt
	benchstat -format csv ./testdata/bench_old.txt ./testdata/bench_new.txt > ./testdata/benchstat_csv_E2E.txt

data-gotestbench:
	@printf '\n== Generate gotestbench E2E fixture: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata/benchtargets > ./testdata/bench_gotestbench.txt

data-insufficient:
	@printf '\n== Generate insufficient raw samples E2E fixture: count=2 asks for more samples ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=2 ./testdata/benchtargets > ./testdata/bench_gotestbench_count2.txt

e2e: e2e-readme-pipelines e2e-benchstat e2e-gotestbench e2e-ab e2e-insufficient

e2e-readme-pipelines: build data
	@printf '\n== Run README pipeline E2E: cached fixture input through README-style pipelines ==\n'
	benchstat ./testdata/bench_old.txt ./testdata/bench_new.txt | $(VERDICT) | tee ./testdata/verdict_readme_benchstat_pipeline_E2E.txt
	cat ./testdata/bench_gotestbench.txt | $(VERDICT) | tee ./testdata/verdict_readme_gotestbench_pipeline_E2E.txt
	grep -Eq 'ExampleFast-[0-9]+: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_readme_benchstat_pipeline_E2E.txt
	grep -q 'BenchmarkEnhance: enhanced wins' ./testdata/verdict_readme_gotestbench_pipeline_E2E.txt

e2e-benchstat: build data e2e-benchstat-text e2e-benchstat-csv

e2e-benchstat-text: e2e-benchstat-text-output e2e-benchstat-text-assert

e2e-benchstat-text-output:
	@printf '\n== Run auto benchstat E2E: repeated BenchmarkExampleFast checks old/new benchstat parsing, not Fast vs Slow speed ==\n'
	$(VERDICT) --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_text_E2E.txt
	$(VERDICT) --verbose --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_verbose_text_E2E.txt
	$(VERDICT) --format json < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_json_E2E.txt

e2e-benchstat-text-assert:
	grep -Eq 'ExampleFast-[0-9]+: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_text_E2E.txt
	grep -Eq '(no statistically significant practical difference|Pareto-superior)' ./testdata/verdict_verbose_text_E2E.txt
	grep -Eq '"benchmark": "ExampleFast-[0-9]+"' ./testdata/verdict_json_E2E.txt

e2e-benchstat-csv: e2e-benchstat-csv-output e2e-benchstat-csv-assert

e2e-benchstat-csv-output:
	$(VERDICT) --format text < ./testdata/benchstat_csv_E2E.txt | tee ./testdata/verdict_csv_text_E2E.txt
	$(VERDICT) --format json < ./testdata/benchstat_csv_E2E.txt | tee ./testdata/verdict_csv_json_E2E.txt

e2e-benchstat-csv-assert:
	grep -Eq 'ExampleFast-[0-9]+: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_csv_text_E2E.txt
	grep -Eq '"benchmark": "ExampleFast-[0-9]+"' ./testdata/verdict_csv_json_E2E.txt

e2e-gotestbench: build data e2e-gotestbench-output e2e-gotestbench-assert

e2e-gotestbench-output:
	@printf '\n== Run auto gotestbench E2E: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced checks quick local comparison ==\n'
	$(VERDICT) --format text < ./testdata/bench_gotestbench.txt | tee ./testdata/verdict_gotestbench_text_E2E.txt
	$(VERDICT) --verbose --format text < ./testdata/bench_gotestbench.txt | tee ./testdata/verdict_gotestbench_verbose_text_E2E.txt
	$(VERDICT) --format json < ./testdata/bench_gotestbench.txt | tee ./testdata/verdict_gotestbench_json_E2E.txt

e2e-gotestbench-assert:
	grep -q 'BenchmarkEnhance: enhanced wins' ./testdata/verdict_gotestbench_text_E2E.txt
	grep -q 'Pareto-superior' ./testdata/verdict_gotestbench_verbose_text_E2E.txt
	grep -q '"benchmark": "BenchmarkEnhance"' ./testdata/verdict_gotestbench_json_E2E.txt

e2e-ab: build data e2e-ab-mismatch e2e-ab-explicit

e2e-ab-mismatch:
	@printf '\n== Run explicit A/B E2E: different benchmark names can be compared with -a and -b ==\n'
	! benchstat ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt | $(VERDICT) 2> ./testdata/verdict_mismatch_error_E2E.txt
	grep -q 'benchmark names differ' ./testdata/verdict_mismatch_error_E2E.txt
	grep -q 'verdict -a' ./testdata/verdict_mismatch_error_E2E.txt

e2e-ab-explicit:
	$(VERDICT) -a ./testdata/bench_ExampleFast.txt -b ./testdata/bench_ExampleSlow.txt | tee ./testdata/verdict_ab_text_E2E.txt
	grep -q 'BenchmarkExampleFast_vs_BenchmarkExampleSlow: BenchmarkExampleFast wins' ./testdata/verdict_ab_text_E2E.txt

e2e-insufficient: build data e2e-insufficient-assert

e2e-insufficient-assert:
	! $(VERDICT) < ./testdata/bench_gotestbench_count2.txt 2> ./testdata/verdict_insufficient_error_E2E.txt
	grep -q 'insufficient samples' ./testdata/verdict_insufficient_error_E2E.txt
	grep -q 'at least 3 samples' ./testdata/verdict_insufficient_error_E2E.txt
	grep -q -- '-count=10 or more' ./testdata/verdict_insufficient_error_E2E.txt

clean-dist:
	@printf '\n== Clean: removing ./dist ==\n'
	@rm -rf ./dist && printf 'Removed ./dist.\n'

clean-testdata:
	@printf '\n== Clean: removing generated testdata/*.txt files ==\n'
	@rm -f ./testdata/*.txt

clean-all: clean-testdata clean-dist
