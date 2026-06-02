# Library Usage

You can use the `verdict` parser and evaluator from Go code.

## Parse Benchstat Input

```go
package main

import (
  "os"

  "github.com/KEINOS/go-verdict/verdict"
)

func main() {
  report, err := verdict.Parse(os.Stdin, verdict.Options{
    Alpha:       0.05,
    MinDeltaPct: 0,
  })
  if err != nil {
    panic(err)
  }

  if err := report.WriteJSON(os.Stdout); err != nil {
    panic(err)
  }
}
```

`verdict.Options{}` uses the default alpha of `0.05`. `verdict.NewOptions()` returns the same safe defaults for callers that prefer explicit setup. If you set `Alpha`, use a finite value greater than `0` and at most `1`; `MinDeltaPct` must be finite and non-negative.

## Parse Raw Go Test Bench Input

Parse raw `go test -bench` input from stdin by selecting explicit sub-benchmark labels:

```go
report, err := verdict.Parse(os.Stdin, verdict.Options{
  Mode:      "gotestbench",
  Baseline:  "original",
  Candidate: "enhanced",
})
if err != nil {
  panic(err)
}
_ = report.WriteText(os.Stdout)
```

## Compare Raw Files

Compare two raw benchmark files as A/B alternatives:

```go
a, err := os.Open("fast.txt")
if err != nil {
  panic(err)
}
defer a.Close()

b, err := os.Open("slow.txt")
if err != nil {
  panic(err)
}
defer b.Close()

report, err := verdict.CompareRawFiles(a, b, verdict.NewOptions())
if err != nil {
  panic(err)
}
_ = report.WriteJSON(os.Stdout)
```

## Use Outcomes In CI

Use report outcomes directly in CI checks:

```go
report, err := verdict.Parse(os.Stdin, verdict.NewOptions())
if err != nil {
  panic(err)
}

for _, item := range report.Verdicts {
  if item.Outcome == verdict.OldWins || item.Outcome == verdict.TradeOff {
    os.Exit(1)
  }
}
```

## Related Documentation

- [go-verdict](README.md): installation and complete CLI help.
- [CLI Details](README_CLI.md): output formats and verdict meanings.
- [Workflow Details](README_WORKFLOWS.md): complete CLI comparison examples.
