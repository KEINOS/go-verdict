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

The E2E tests build the `verdict` CLI, prepare benchmark fixtures, and run YAML scenarios from `testdata/e2e-scenarios/`.

For normal validation, use:

```sh
make e2e
```

`make e2e` is an alias for `make test-e2e`. It sets the required environment variables for the Go E2E harness:

- `VERDICT_BIN`: path to the built `verdict` binary.
- `VERDICT_E2E_SCENARIOS_DIR`: path to the YAML scenario directory.

To call the Go test directly, build first and pass the `e2e` build tag with those variables:

```sh
make build data
VERDICT_BIN="$(pwd)/dist/verdict" \
VERDICT_E2E_SCENARIOS_DIR="$(pwd)/testdata/e2e-scenarios" \
go test -tags=e2e -race ./tools/e2e/...
```

Add or update YAML scenarios when a CLI workflow changes. Scenario files use repository-root-relative paths for command arguments and `stdin_file` values.

For a clean E2E run, use:

```sh
make clean && make e2e
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
make clean           # remove both
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
testdata/e2e-scenarios/        YAML E2E scenarios for the built CLI
tools/e2e/                     Go E2E harness with the e2e build tag
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
