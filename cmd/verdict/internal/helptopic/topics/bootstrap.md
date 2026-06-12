# Bootstrap A Benchmark

Use this topic when no benchmark exists, or when the existing benchmarks do not cover the code you must optimize.

A verdict is only as good as the benchmark behind it. Build representative evidence first, then optimize.

## Steps

1. Find the user-visible workflow. Tests, fixtures, README examples, CLI behavior, and E2E flows show what the code really does.
2. Benchmark a public entrypoint with realistic input. Avoid trivial or synthetic paths unless the user asked for that exact path.
3. Add one sub-benchmark per realistic scenario, such as short, long, and worst-case inputs.
4. Always run with `-benchmem` so allocation metrics join the decision.

## Template

```go
func BenchmarkParse(b *testing.B) {
    input := loadRealisticFixture()

    b.ReportAllocs()
    b.ResetTimer()

    for b.Loop() {
        _ = Parse(input)
    }
}
```

Verify the benchmark produces rows before relying on it:

```sh
go test -run='^$' -bench=. -benchmem ./your/package
```

## Checklist

- The benchmark calls the public entrypoint users call, not a private helper picked for convenience.
- Inputs match real usage in size and shape.
- For CLI tools, benchmark the public runner with realistic stdin, args, and writers.
- Ask the maintainer only when the workflow cannot be inferred or the benchmark needs an out-of-scope behavior change.

## Next

- Find the function to optimize: `verdict help hotspot`.
- Judge a before/after change: `verdict help benchstat`.
