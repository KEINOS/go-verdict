.PHONY: test lint build data data-example-mismatch data-benchstat-repeat-fast data-alternatives e2e e2e-benchstat e2e-alternatives e2e-ab e2e-insufficient

VERDICT := ./dist/verdict

test:
	go test -v -cover -race ./...

lint:
	go fix ./...
	golangci-lint run --fix --timeout 5m
	markdownlint-cli2 --fix "**/*.md"

build:
	mkdir -p ./dist
	go build -o $(VERDICT) ./cmd/verdict/main.go

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

e2e: e2e-benchstat e2e-alternatives e2e-ab e2e-insufficient

e2e-benchstat: build data-benchstat-repeat-fast
	@printf '\n== Run auto benchstat E2E: repeated BenchmarkExampleFast checks old/new benchstat parsing, not Fast vs Slow speed ==\n'
	$(VERDICT) --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_text_E2E.txt
	$(VERDICT) --verbose --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_verbose_text_E2E.txt
	$(VERDICT) --format json < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_json_E2E.txt
	grep -Eq 'ExampleFast-10: (tie|bench_(old|new)\.txt wins)' ./testdata/verdict_text_E2E.txt
	grep -Eq '(no statistically significant practical difference|Pareto-superior)' ./testdata/verdict_verbose_text_E2E.txt
	grep -q '"benchmark": "ExampleFast-10"' ./testdata/verdict_json_E2E.txt

e2e-alternatives: build data-alternatives
	@printf '\n== Run auto alternatives E2E: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced checks local PoC comparison ==\n'
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
	grep -q -- '-count=10 or more' ./testdata/verdict_insufficient_error_E2E.txt
