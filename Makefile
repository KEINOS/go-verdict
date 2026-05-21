.PHONY: help test lint build clean-dist clean-testdata clean-all data data-example-mismatch data-benchstat-repeat-fast data-alternatives e2e e2e-readme-pipelines e2e-benchstat e2e-alternatives e2e-ab e2e-insufficient

VERDICT := ./dist/verdict

help:
	@printf 'Development targets (run from the repository root):\n'
	@printf '  make test                 Run unit tests with race detector and coverage.\n'
	@printf '  make lint                 Run mutating fixers: go fix, golangci-lint --fix, markdownlint-cli2 --fix.\n'
	@printf '  make build                Remove ./dist, then build ./dist/verdict.\n'
	@printf '  make clean-dist           Remove ./dist.\n'
	@printf '  make clean-testdata       Remove generated testdata/*.txt files.\n'
	@printf '  make clean-all            Remove generated testdata/*.txt files and ./dist.\n'
	@printf '\nFixture targets:\n'
	@printf '  make data                 Regenerate benchmark fixtures in testdata/.\n'
	@printf '  make data-alternatives    Regenerate the raw alternatives fixture.\n'
	@printf '\nEnd-to-end targets (build first and assume repo-root paths):\n'
	@printf '  make e2e                  Run all E2E checks.\n'
	@printf '  make e2e-benchstat        Check benchstat stdin parsing.\n'
	@printf '  make e2e-alternatives     Check raw alternatives stdin parsing.\n'
	@printf '  make e2e-ab               Check explicit raw-file A/B comparison.\n'
	@printf '  make e2e-insufficient     Check insufficient raw sample guidance.\n'

test:
	go test -cover -race ./...

test-verbose:
	go test -v -cover -race ./...

lint:
	go fix ./...
	golangci-lint run --fix --timeout 5m
	markdownlint-cli2 --fix "**/*.md"

build: clean-dist
	@printf '\n== Build: compiling ./cmd/verdict to ./dist/verdict ==\n'
	@mkdir -p ./dist
	@go build -ldflags="-s -w" -trimpath -o $(VERDICT) ./cmd/verdict
	@$(VERDICT) -v 1> /dev/null && \
		$(VERDICT) --version 1> /dev/null && \
		$(VERDICT) version 1> /dev/null && \
		$(VERDICT) -h 1> /dev/null && \
		printf 'Built ./dist/verdict successfully.\n' || \
		(printf 'Failed to build ./dist/verdict.\n' && exit 1)

data: data-example-mismatch data-benchstat-repeat-fast data-alternatives

data-example-mismatch:
	@printf '\n== Generate example fixture: different benchmark names, useful for mismatch examples ==\n'
	go test -bench=BenchmarkExampleFast -count=10 ./testdata | tee ./testdata/bench_ExampleFast.txt
	go test -bench=BenchmarkExampleSlow -count=10 ./testdata | tee ./testdata/bench_ExampleSlow.txt
	benchstat ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt > ./testdata/benchstat_Example.txt

data-benchstat-repeat-fast:
	@printf '\n== Generate benchstat E2E fixture: repeat BenchmarkExampleFast as old/new, with no intentional implementation difference ==\n'
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata > ./testdata/bench_old.txt
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata > ./testdata/bench_new.txt
	benchstat ./testdata/bench_old.txt ./testdata/bench_new.txt > ./testdata/benchstat_E2E.txt

data-alternatives:
	@printf '\n== Generate alternatives E2E fixture: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata > ./testdata/bench_alternatives.txt

e2e: e2e-readme-pipelines e2e-benchstat e2e-alternatives e2e-ab e2e-insufficient

e2e-readme-pipelines: build data-benchstat-repeat-fast
	@printf '\n== Run README pipeline E2E: literal pipeline examples from the README opening ==\n'
	benchstat ./testdata/bench_old.txt ./testdata/bench_new.txt | $(VERDICT) | tee ./testdata/verdict_readme_benchstat_pipeline_E2E.txt
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata | $(VERDICT) | tee ./testdata/verdict_readme_alternatives_pipeline_E2E.txt
	grep -Eq 'ExampleFast-10: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_readme_benchstat_pipeline_E2E.txt
	grep -q 'BenchmarkEnhance: enhanced wins' ./testdata/verdict_readme_alternatives_pipeline_E2E.txt

e2e-benchstat: build data-benchstat-repeat-fast
	@printf '\n== Run auto benchstat E2E: repeated BenchmarkExampleFast checks old/new benchstat parsing, not Fast vs Slow speed ==\n'
	$(VERDICT) --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_text_E2E.txt
	$(VERDICT) --verbose --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_verbose_text_E2E.txt
	$(VERDICT) --format json < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_json_E2E.txt
	grep -Eq 'ExampleFast-10: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_text_E2E.txt
	grep -Eq '(no statistically significant practical difference|Pareto-superior)' ./testdata/verdict_verbose_text_E2E.txt
	grep -q '"benchmark": "ExampleFast-10"' ./testdata/verdict_json_E2E.txt

e2e-alternatives: build data-alternatives
	@printf '\n== Run auto alternatives E2E: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced checks quick local comparison ==\n'
	$(VERDICT) --format text < ./testdata/bench_alternatives.txt | tee ./testdata/verdict_alternatives_text_E2E.txt
	$(VERDICT) --verbose --format text < ./testdata/bench_alternatives.txt | tee ./testdata/verdict_alternatives_verbose_text_E2E.txt
	$(VERDICT) --format json < ./testdata/bench_alternatives.txt | tee ./testdata/verdict_alternatives_json_E2E.txt
	grep -q 'BenchmarkEnhance: enhanced wins' ./testdata/verdict_alternatives_text_E2E.txt
	grep -q 'Pareto-superior' ./testdata/verdict_alternatives_verbose_text_E2E.txt
	grep -q '"benchmark": "BenchmarkEnhance"' ./testdata/verdict_alternatives_json_E2E.txt

e2e-ab: build data-example-mismatch
	@printf '\n== Run explicit A/B E2E: different benchmark names can be compared with -a and -b ==\n'
	! benchstat ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt | $(VERDICT) 2> ./testdata/verdict_mismatch_error_E2E.txt
	grep -q 'benchmark names differ' ./testdata/verdict_mismatch_error_E2E.txt
	grep -q 'verdict -a' ./testdata/verdict_mismatch_error_E2E.txt
	$(VERDICT) -a ./testdata/bench_ExampleFast.txt -b ./testdata/bench_ExampleSlow.txt | tee ./testdata/verdict_ab_text_E2E.txt
	grep -q 'BenchmarkExampleFast_vs_BenchmarkExampleSlow: BenchmarkExampleFast wins' ./testdata/verdict_ab_text_E2E.txt

e2e-insufficient: build
	@printf '\n== Run insufficient raw samples E2E: count=2 asks for more samples ==\n'
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=2 ./testdata > ./testdata/bench_alternatives_count2.txt
	! $(VERDICT) < ./testdata/bench_alternatives_count2.txt 2> ./testdata/verdict_insufficient_error_E2E.txt
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
