# Contributing

Thanks for improving `go-verdict`. This project is intentionally small, so
the safest changes are usually narrow ones that keep parser behavior,
evaluation rules, fixtures, and documentation in sync.

## Repository Assumptions

Run project commands from the repository root. The Makefile, Markdown lint
configuration, fixture paths, and end-to-end checks assume the current working
directory contains `go.mod`, `Makefile`, and `.markdownlint-cli2.yaml`.

Start by checking the worktree:

```sh
git status --short
```

Avoid overwriting unrelated local changes. If a fixture, generated file, or
documentation file is already modified, understand whether it belongs to your
change before editing it.

## Safe Development Workflow

Use focused checks while iterating:

```sh
go test ./...
```

Run the full race and coverage test target before larger changes:

```sh
make test
```

Run lint only when you are ready for mutating fixers:

```sh
make lint
```

`make lint` runs `go fix`, `golangci-lint --fix`, and
`markdownlint-cli2 --fix`. It can rewrite Go and Markdown files. Run it from
the repository root so the project configuration is applied.

## End-to-End Checks

The E2E targets build `./dist/verdict`, generate or use files in `testdata/`,
and assert the command output with `grep`.

```sh
make e2e
make e2e-benchstat
make e2e-alternatives
make e2e-ab
make e2e-insufficient
```

Use the focused E2E target that matches your change. For example, raw
alternatives changes usually need `make e2e-alternatives`, while explicit raw
file comparison changes usually need `make e2e-ab`.

## Fixture Regeneration

`make data` regenerates benchmark fixtures under `testdata/`:

```sh
make data
```

Only regenerate fixtures when benchmark examples, fixture shape, or E2E
expectations intentionally change. Benchmark data contains normal measurement
variance, so regenerated files can differ even when behavior did not change.
Review fixture diffs carefully before committing them.

## Architecture Overview

`go-verdict` has three input paths that converge on the same verdict
evaluation and output writers:

1. Benchstat stdin: `verdict.Parse` reads `benchstat` text or CSV output,
   parsed by `benchstat_text.go` or `benchstat_csv.go`.
2. Raw alternatives stdin: auto mode recognizes repeated `go test -bench`
   rows such as `BenchmarkName/original` and `BenchmarkName/enhanced`, parsed
   by `raw.go`.
3. Raw-file A/B comparison: the CLI `-a` and `-b` flags call
   `verdict.CompareRawFiles`, also implemented in `raw.go`, for two separate
   raw benchmark files.

All paths produce `Comparison` rows and then use the shared evaluator in the
`verdict` package to produce a `Report`. Text, verbose text, and JSON output
are written by methods on `Report`.

The CLI lives in `cmd/verdict/`. The public library API lives in `verdict/`.
The embedded AI Agent skill text lives in `skill/verdict/`.

## Error Message Style

Keep user-facing errors actionable and specific. Existing patterns distinguish
library reports from CLI errors:

- Library reports may return `inconclusive` with a `ReasonCode`.
- The CLI turns selected cases into direct errors when the user needs to take
  action, such as benchmark-set mismatch or insufficient raw samples.
- Insufficient-sample guidance should mention the minimum accepted sample count
  and recommend `-count=10` or more.
- Benchmark-set mismatch guidance should point users to `verdict -a` and `-b`
  when they are comparing different benchmark functions.

Do not broaden error-message behavior casually. Add or adjust tests whenever
the user-visible message or reason code changes.

## Adding An Input Or Output Format

Use this checklist when extending what `verdict` can read or write:

- Decide whether the new format is benchstat-like, raw-benchmark-like, or a
  separate parser path.
- Add parser code in the `verdict` package without changing existing mode
  behavior unless the new format is intentionally part of auto detection.
- Convert parsed data into `Comparison` rows so shared verdict evaluation still
  owns `Direction`, `Outcome`, `Reason`, and winner selection.
- Update CLI mode or format validation in `cmd/verdict/` if users need a new
  flag value.
- Add unit tests for parser success, malformed input, unsupported metrics, and
  inconclusive cases.
- Add or update E2E coverage when the CLI workflow changes.
- Update README usage, CLI option text, and this contributor guide.
- Regenerate fixtures only when fixture contents are intentionally part of the
  change.
- Run Markdown lint on changed Markdown files from the repository root.

## Markdown

After Markdown edits, run:

```sh
markdownlint-cli2 --fix README.md CONTRIBUTING.md tasks.md plan.md
```

Limit the file list to the Markdown files you changed when possible. This keeps
unrelated documentation churn out of the patch.
