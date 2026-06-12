# Library Usage

You can use the `verdict` parser and evaluator in your go code.

## Parse Benchstat Input

```go
package main

import (
  "os"

  "github.com/KEINOS/go-verdict/verdict"
)

func main() {
  opt := verdict.NewOptions()
  opt.Alpha = 0.1     // default is 0.05, use a finite value greater than 0 and at most 1
  opt.MinDeltaPct = 0 // default is 0, must be finite and non-negative

  report, err := verdict.Parse(os.Stdin, opt)
  if err != nil {
    panic(err)
  }

  err = report.WriteJSON(os.Stdout)
  if err != nil {
    panic(err)
  }
}
```

- `verdict.NewOptions()` returns the same safe defaults as the CLI:
  - verdict.Options.Alpha = 0.05
  - verdict.Options.MinDeltaPct = 0
  - verdict.Options.Mode = verdict.ModeAuto
  - verdict.Options.Baseline = ""
  - verdict.Options.Candidate = ""
- If you set `Alpha`, use a finite value greater than `0` and at most `1`; `MinDeltaPct` must be finite and non-negative.
- Supported mode constants are `verdict.ModeAuto`, `verdict.ModeBenchstat`, and `verdict.ModeGoTestBench`.

## Parse Raw Go Test Bench Input

Parse raw `go test -bench` input from stdin by selecting explicit sub-benchmark labels:

```go
report, err := verdict.Parse(os.Stdin, verdict.Options{
  Alpha:       0.1,
  MinDeltaPct: 5,
  Mode:        verdict.ModeGoTestBench,
  Baseline:    "original",
  Candidate:   "enhanced",
})
if err != nil {
  panic(err)
}

err = report.WriteText(os.Stdout)
if err != nil {
  panic(err)
}
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

err = report.WriteJSON(os.Stdout)
if err != nil {
  panic(err)
}
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

## Use Inconclusive Reason Codes

`BenchmarkVerdict.ReasonCode` is a string so future versions can add reason codes without changing the report type. For known reason codes, use the public constants:

```go
for _, item := range report.Verdicts {
  if item.Outcome == verdict.Inconclusive && item.ReasonCode == verdict.ReasonInsufficientSamples {
    os.Exit(1)
  }
}
```

## Related Documentation

- [go-verdict](README.md): installation and complete CLI help.
- [CLI Details](README_CLI.md): output formats and verdict meanings.
- [Workflow Details](README_WORKFLOWS.md): complete CLI comparison examples.
