.PHONY: test lint build data data-example-mismatch data-benchstat-repeat-fast data-alternatives e2e e2e-benchstat e2e-alternatives

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

e2e: e2e-benchstat e2e-alternatives

e2e-benchstat: build data-benchstat-repeat-fast
	@printf '\n== Run benchstat-mode E2E: repeated BenchmarkExampleFast checks old/new benchstat parsing, not Fast vs Slow speed ==\n'
	$(VERDICT) --format text < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_text_E2E.txt
	$(VERDICT) --format json < ./testdata/benchstat_E2E.txt | tee ./testdata/verdict_json_E2E.txt
	grep -q 'ExampleFast-10:' ./testdata/verdict_text_E2E.txt
	grep -q '"benchmark": "ExampleFast-10"' ./testdata/verdict_json_E2E.txt

e2e-alternatives: build data-alternatives
	@printf '\n== Run alternatives-mode E2E: BenchmarkEnhance/original vs BenchmarkEnhance/enhanced checks local PoC comparison ==\n'
	$(VERDICT) --mode alternatives --format text < ./testdata/bench_alternatives.txt | tee ./testdata/verdict_alternatives_text_E2E.txt
	$(VERDICT) --mode alternatives --format json < ./testdata/bench_alternatives.txt | tee ./testdata/verdict_alternatives_json_E2E.txt
	grep -q 'BenchmarkEnhance:' ./testdata/verdict_alternatives_text_E2E.txt
	grep -q '"benchmark": "BenchmarkEnhance"' ./testdata/verdict_alternatives_json_E2E.txt
