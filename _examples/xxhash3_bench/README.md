# Compare xxHash3 Implementations

This example benchmarks `github.com/cespare/xxhash/v2` and `github.com/zeebo/xxh3`.

```shellsession
$ verdict --version
v1.0.0-beta.0.20261101074022-7f9c9a16cdc8 (unknown)

$ go test -run='^$' -bench=. -benchmem -count=10 ./... | verdict
BenchmarkCounter: zeebo wins
BenchmarkLargeChunk: zeebo wins
```
