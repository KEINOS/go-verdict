.PHONY: test lint data e2e

test:
	go test -v -cover -race ./...

lint:
	go fix ./...
	golangci-lint run --fix --timeout 5m
	markdownlint-cli2 --fix "**/*.md"

data:
	go test -bench=BenchmarkExampleFast -count=10 ./testdata | tee ./testdata/bench_ExampleFast.txt
	go test -bench=BenchmarkExampleSlow -count=10 ./testdata | tee ./testdata/bench_ExampleSlow.txt
	benchstat ./testdata/bench_ExampleFast.txt ./testdata/bench_ExampleSlow.txt > ./testdata/benchstat_Example.txt
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata > ./testdata/bench_old.txt
	go test -run='^$$' -bench=BenchmarkExampleFast -count=12 ./testdata > ./testdata/bench_new.txt
	benchstat ./testdata/bench_old.txt ./testdata/bench_new.txt > ./testdata/benchstat_E2E.txt
	go test -run='^$$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata > ./testdata/bench_alternatives.txt

e2e: data
	mkdir -p ./dist
	go build -o ./dist/verdict ./cmd/verdict/main.go
	cat ./testdata/benchstat_E2E.txt | ./dist/verdict --format text | tee ./testdata/verdict_text_E2E.txt
	cat ./testdata/benchstat_E2E.txt | ./dist/verdict --format json | tee ./testdata/verdict_json_E2E.txt
	cat ./testdata/bench_alternatives.txt | ./dist/verdict --mode alternatives --format text | tee ./testdata/verdict_alternatives_text_E2E.txt
	cat ./testdata/bench_alternatives.txt | ./dist/verdict --mode alternatives --format json | tee ./testdata/verdict_alternatives_json_E2E.txt
	grep -q 'ExampleFast-10:' ./testdata/verdict_text_E2E.txt
	grep -q '"benchmark": "ExampleFast-10"' ./testdata/verdict_json_E2E.txt
	grep -q 'BenchmarkEnhance:' ./testdata/verdict_alternatives_text_E2E.txt
	grep -q '"benchmark": "BenchmarkEnhance"' ./testdata/verdict_alternatives_json_E2E.txt
