# Contributing

Thanks for improving `go-verdict`. Keep changes small. When behavior changes, update tests, fixtures, and documentation together.

## Contents

- [Before You Start](#before-you-start)
- [Development Commands](#development-commands)
- [End-to-End Checks](#end-to-end-checks)
- [Benchmark Fixtures](#benchmark-fixtures)
- [Architecture And Layout](#architecture-and-layout)
- [Error Message Style](#error-message-style)
- [Adding An Input Or Output Format](#adding-an-input-or-output-format)
- [Markdown](#markdown)

## Before You Start

Run commands from the repository root. The project tools expect to find `go.mod`, `Makefile`, and `.markdownlint-cli2.yaml` in the current directory.

Check the worktree before editing:

```sh
git status --short
```

Do not overwrite unrelated local changes. Review existing changes before editing a modified file.

## Development Commands

- `go test ./...`: Run focused tests while iterating.
- `make test`: Run all tests with the race detector and coverage.
- `make lint`: Run mutating fixers. It may rewrite Go and Markdown files.
- `make build`: Remove `./dist`, then build `./dist/verdict`.
- `make check`: Run `make test`, `make lint`, and `make e2e`.
- `make help`: List development targets.

## End-to-End Checks

The E2E tests build the `verdict` CLI and check its output using files in `testdata/` (benchmark fixtures).

Use the E2E target that matches your change:

```sh
make e2e
make e2e-benchstat
make e2e-gotestbench
make e2e-ab
make e2e-insufficient
```

For a clean E2E run, use:

```sh
make clean-all && make e2e
```

## Benchmark Fixtures

Regenerate benchmark fixtures only when benchmark examples, fixture formats, or E2E expectations change, or when fixtures are not present in `testdata/`.

```sh
make data
```

Benchmark results include normal measurement variance (acceptable variations in the values). Review fixture diffs before committing them.

Cleanup targets:

```sh
make clean-dist      # remove ./dist
make clean-testdata  # remove generated testdata/*.txt files
make clean-all       # remove both
```

## Architecture And Layout

1. All input paths create `Comparison` rows.
2. The shared evaluator in `verdict/` turns them into a `Report`.
3. `Report` methods write text, verbose text, and JSON output.

Supported input paths:

1. Benchstat stdin: `verdict.Parse` reads benchstat text or CSV.
2. Raw `go test -bench` stdin: `auto` mode (default) reads repeated sub-benchmarks such as `BenchmarkName/original` and `BenchmarkName/enhanced`.
3. Raw-file A/B comparison: CLI flags `-a` and `-b` call `verdict.CompareRawFiles`.

Project layout:

```text
cmd/verdict/                   CLI entry point
cmd/verdict/internal/skill/    Embedded AI Agent skill text
verdict/                       Public API, evaluator, and output writer
verdict/internal/benchparser/  Benchmark input decoders
testdata/                      Demo benchmarks and generated fixtures
Makefile                       Fixture/jig and E2E commands
```

## Error Message Style

Keep user-facing errors specific and actionable.

- Library reports may return `inconclusive` with a `ReasonCode`.
- The CLI returns direct errors when the user must take action, such as benchmark-set mismatch or insufficient raw samples.
- Insufficient-sample errors should state the minimum sample count and recommend `-count=10` or more.
- Benchmark-set mismatch errors should point users to `verdict -a` and `-b` for different benchmark functions.

Update tests when a user-facing message or reason code changes.

## Adding An Input Or Output Format

When adding a format:

- Choose an existing parser path or add a separate path under `verdict/internal/benchparser/`. Keep existing mode behavior unless the new format is intentionally part of auto-detection.
- Keep shared thresholds, statistics, outcomes, reasons, and winner selection in the `verdict` package.
- Update CLI validation in `cmd/verdict/` when users need a new flag value.
- Add unit tests for valid input, malformed input, unsupported metrics, and inconclusive cases.
- Add or update E2E checks when the CLI workflow changes.
- Update README files, CLI option text, and this guide when needed.
- Regenerate fixtures only when fixture contents must change.
- Update `make help` and this guide when Makefile targets or generated paths change.

## Markdown

After editing Markdown, lint only the files you changed when possible:

```sh
markdownlint-cli2 --fix CONTRIBUTING.md
```

### No Soft Break

Avoid soft breaks inside prose sentences and list items. Soft breaks can change the rendered layout.

If a raw Markdown line feels too long, shorten the sentence instead of wrapping it.
